package cmd

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ze-mu-zhou/SFR-ipgw/pkg/handler"
	"github.com/urfave/cli/v2"
)

type fakeDashboard struct {
	billErr error
	calls   int
}

func (f *fakeDashboard) GetBasic() (*handler.Basic, error) {
	f.calls++
	return &handler.Basic{ID: "test-user", Name: "Test"}, nil
}
func (f *fakeDashboard) GetPackage() (*handler.Package, error) {
	f.calls++
	return &handler.Package{UsedTraffic: "1.00 GB"}, nil
}
func (f *fakeDashboard) GetDevice() ([]handler.Device, error) {
	f.calls++
	return []handler.Device{}, nil
}
func (f *fakeDashboard) GetRecharge(int) ([]handler.RechargeRecord, error) {
	f.calls++
	return []handler.RechargeRecord{}, nil
}
func (f *fakeDashboard) GetUsageRecords(int) ([]handler.UsageRecord, error) {
	f.calls++
	return []handler.UsageRecord{}, nil
}
func (f *fakeDashboard) GetBill(int) ([]handler.BillRecord, error) {
	f.calls++
	return []handler.BillRecord{}, f.billErr
}

func infoContext(t *testing.T, args ...string) *cli.Context {
	t.Helper()
	set := flag.NewFlagSet("info", flag.ContinueOnError)
	for _, f := range InfoCommand.Flags {
		if err := f.Apply(set); err != nil {
			t.Fatal(err)
		}
	}
	if err := set.Parse(args); err != nil {
		t.Fatal(err)
	}
	app := cli.NewApp()
	app.Writer = io.Discard
	app.ErrWriter = io.Discard
	return cli.NewContext(app, set, nil)
}

func TestPartialQueryFailureAndEmptyRecords(t *testing.T) {
	reader := &fakeDashboard{billErr: errors.New("network unavailable")}
	report := &infoReport{Sections: map[string]querySection{}}
	err := collectInfo(infoContext(t, "--all"), reader, report)
	if err == nil || reader.calls != 6 {
		t.Fatalf("must report error and query other sections: %v %d", err, reader.calls)
	}
	if report.Sections["bills"].Status != "error" || report.Sections["usage"].Status != "ok" {
		t.Fatal("error and empty records not distinguished")
	}
	var output bytes.Buffer
	printInfoReport(&output, report)
	if !strings.Contains(output.String(), "network unavailable") || !strings.Contains(output.String(), "无记录") {
		t.Fatal("missing useful failure or empty message")
	}
	if _, err := snapshotFromReport(report, "2026-09", 1000); err == nil {
		t.Fatal("snapshot created from failed query")
	}
}

func TestStoredAccountUsedWithoutPrompt(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"accounts":[{"username":"test-user","credential_ref":"ipgw/account/test"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := accountContext(t, configPath, "--username", "test-user")
	a, err := getAccountByContext(ctx)
	if err != nil || a.Username != "test-user" || a.CredentialRef != "ipgw/account/test" {
		t.Fatalf("stored account not used: %v", err)
	}
}

func TestUnknownAccountRequiresInteractivePassword(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "missing-parent", "config.json")
	ctx := accountContext(t, configPath, "--username", "unknown-user")
	if _, err := getAccountByContext(ctx); err == nil {
		t.Fatal("unknown account without terminal accepted")
	}
}

func accountContext(t *testing.T, configPath string, args ...string) *cli.Context {
	t.Helper()
	ctx := infoContext(t)
	set := flag.NewFlagSet("global", flag.ContinueOnError)
	set.String("config", configPath, "")
	parent := cli.NewContext(ctx.App, set, nil)
	local := flag.NewFlagSet("info", flag.ContinueOnError)
	for _, f := range InfoCommand.Flags {
		if err := f.Apply(local); err != nil {
			t.Fatal(err)
		}
	}
	if err := local.Parse(args); err != nil {
		t.Fatal(err)
	}
	return cli.NewContext(ctx.App, local, parent)
}

func TestSnapshotFileDoesNotOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeNewJSON(path, map[string]int{"new": 1}); err == nil {
		t.Fatal("overwrote snapshot")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "keep" {
		t.Fatal("file changed")
	}
}

func TestSnapshotPeriodSource(t *testing.T) {
	r := &infoReport{CompletedAt: time.Now().UTC(), Sections: map[string]querySection{}}
	if err := collectInfo(infoContext(t, "--package"), &fakeDashboard{}, r); err != nil {
		t.Fatal(err)
	}
	s, err := snapshotFromReport(r, "", 1000)
	if err != nil || s.PeriodSource != "unknown" || s.BillingPeriod != "" {
		t.Fatalf("guessed cycle: %+v %v", s, err)
	}
	s, err = snapshotFromReport(r, "2026-09", 1000)
	if err != nil || s.PeriodSource != "user" {
		t.Fatalf("user cycle not marked: %+v %v", s, err)
	}
	r.Sections["package"].Data.(*handler.Package).BillingPeriod = "2026-08"
	if _, err = snapshotFromReport(r, "2026-09", 1000); err == nil {
		t.Fatal("contradictory cycle accepted")
	}
}

func TestInvalidOptionsProduceJSONFailure(t *testing.T) {
	ctx := infoContext(t, "--json", "--log", "0")
	var output bytes.Buffer
	ctx.App.Writer = &output
	if err := runInfo(ctx); err == nil {
		t.Fatal("invalid page succeeded")
	}
	if !strings.Contains(output.String(), `"status": "error"`) || !strings.Contains(output.String(), "页码必须大于零") {
		t.Fatalf("missing JSON error: %s", output.String())
	}
}

func TestReadSnapshotRejectsTrailingData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte(`{} {}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSnapshot(path); err == nil {
		t.Fatal("accepted trailing JSON")
	}
}
