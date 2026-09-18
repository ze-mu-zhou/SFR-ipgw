package cmd

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/urfave/cli/v2"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/console"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/model"
)

// Keep usage errors in-process so the tests can check side effects as well as
// the returned error. Production main turns the same errors into exit code 1.
func argumentTestApp(w io.Writer) *cli.App {
	usageError := func(_ *cli.Context, err error, _ bool) error { return err }
	var clone func(*cli.Command) *cli.Command
	clone = func(command *cli.Command) *cli.Command {
		copy := *command
		copy.OnUsageError = usageError
		copy.Subcommands = nil
		for _, child := range command.Subcommands {
			copy.Subcommands = append(copy.Subcommands, clone(child))
		}
		return &copy
	}
	app := &cli.App{Name: "ipgw", Writer: w, ErrWriter: w, Flags: App.Flags, Action: App.Action, OnUsageError: usageError, HideVersion: true}
	for _, command := range App.Commands {
		app.Commands = append(app.Commands, clone(command))
	}
	return app
}

type argumentTestTransport struct{ calls int }

func (t *argumentTestTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.calls++
	return nil, errors.New("unexpected network request")
}

func TestInvalidArgumentsHaveNoSideEffects(t *testing.T) {
	transport := &argumentTestTransport{}
	oldTransport := http.DefaultTransport
	http.DefaultTransport = transport
	stdin, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	oldStdin, oldOut, oldErr := os.Stdin, console.Stdout, console.Stderr
	os.Stdin = stdin
	var consoleOutput bytes.Buffer
	console.Stdout, console.Stderr = &consoleOutput, &consoleOutput
	t.Cleanup(func() {
		http.DefaultTransport = oldTransport
		os.Stdin, console.Stdout, console.Stderr = oldStdin, oldOut, oldErr
		stdin.Close()
	})

	commands := [][]string{
		{"login", "-u", "review"}, {"logout"}, {"test"}, {"update"}, {"version"},
		{"info", "--json"},
		{"config", "account", "add", "-u", "new", "--no-store-password"},
		{"config", "account", "del", "-u", "review"},
		{"config", "account", "set", "-u", "review", "--default"},
		{"config", "account", "list"}, {"config", "account", "ls"},
	}
	tails := [][]string{{"typo"}, {"typo", "--dry-run"}, {"typo", "--help"}, {"--", "typo"}, {"--dry-run"}}
	var cases [][]string
	for _, command := range commands {
		for _, tail := range tails {
			cases = append(cases, append(append([]string{}, command...), tail...))
		}
	}
	cases = append(cases,
		[]string{"typo"}, []string{"config", "typo"}, []string{"config", "account", "typo"},
		[]string{"config", "account", "del"},
		[]string{"kick"}, []string{"kick", "sid", "--dry-run"},
		[]string{"kick", "sid", "-u", "review"}, []string{"kick", "sid", "--ask-password"},
		[]string{"kick", "sid", "--help"}, []string{"kick", "sid", ""}, []string{"kick", "sid", "  "},
		[]string{"kick", "sid", "--", "--dry-run"},
		[]string{"compare"}, []string{"compare", "before.json"},
		[]string{"compare", "before.json", "after.json", "extra.json"},
		[]string{"compare", "before.json", "--json"},
		[]string{"compare", "before.json", "after.json", "--json"},
	)
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			original := []byte(`{"default_account":"other","accounts":[{"username":"review"},{"username":"other"}]}`)
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			consoleOutput.Reset()
			transport.calls = 0
			err := argumentTestApp(&output).Run(append([]string{"ipgw", "--config", path}, args...))
			if err == nil {
				t.Fatal("invalid input succeeded")
			}
			if !strings.Contains(err.Error(), "参数") && !strings.Contains(err.Error(), "用法") && !strings.Contains(err.Error(), "SID") && !strings.Contains(err.Error(), "flag") {
				t.Fatalf("input reached business logic: %v", err)
			}
			if transport.calls != 0 {
				t.Fatalf("made %d network requests", transport.calls)
			}
			if consoleOutput.Len() != 0 {
				t.Fatalf("business output or password prompt: %s", consoleOutput.String())
			}
			data, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(data, original) {
				t.Fatalf("configuration changed: %s, %v", data, err)
			}
			if args[0] == "info" && args[len(args)-1] != "--dry-run" && !strings.Contains(output.String(), `"status": "error"`) {
				t.Fatalf("info lost structured error output: %s", output.String())
			}
		})
	}
}

func TestValidArgumentForms(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	for _, args := range [][]string{
		{"config"}, {"config", "account"}, {"config", "account", "--help"},
		{"config", "account", "add", "-u", "review", "--no-store-password", "--default"},
		{"config", "account", "set", "-u", "review", "--default"},
		{"config", "account", "ls"}, {"version"}, {"version", "--"},
		{"config", "account", "del", "-u", "review"},
	} {
		if err := argumentTestApp(io.Discard).Run(append([]string{"ipgw", "--config", path}, args...)); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	beforePath, afterPath := filepath.Join(t.TempDir(), "-before.json"), filepath.Join(t.TempDir(), "after.json")
	measurement, err := model.ParseTraffic("1 B", 0)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := model.Snapshot{SchemaVersion: 1, AccountID: "review", CapturedAt: time.Now().UTC(), QueryStatus: "ok", Source: "ipgw-dashboard", BillingPeriod: "2026-09", PeriodSource: "user", Traffic: measurement}
	if err := writeNewJSON(beforePath, snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.CapturedAt = snapshot.CapturedAt.Add(time.Second)
	if err := writeNewJSON(afterPath, snapshot); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"compare", "--json", beforePath, afterPath}, {"compare", "--json", "--", beforePath, afterPath}} {
		var output bytes.Buffer
		if err := argumentTestApp(&output).Run(append([]string{"ipgw"}, args...)); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), `"result": "display_unchanged"`) {
			t.Fatalf("unexpected result: %s", output.String())
		}
	}
	// Explicit -- also permits a relative filename that looks like an option.
	t.Chdir(filepath.Dir(beforePath))
	var output bytes.Buffer
	if err := argumentTestApp(&output).Run([]string{"ipgw", "compare", "--json", "--", "-before.json", afterPath}); err != nil {
		t.Fatal(err)
	}
}

func TestKickValidatesWholeSIDListBeforeAction(t *testing.T) {
	for _, tc := range []struct {
		args, want []string
	}{
		{[]string{"kick", "-u", "review", "sid1", "sid2"}, []string{"sid1", "sid2"}},
		{[]string{"kick", "--ask-password", "--", "sid1", "sid2"}, []string{"sid1", "sid2"}},
		{[]string{"kick", "--", "-sid", "--literal-sid"}, []string{"-sid", "--literal-sid"}},
	} {
		app := argumentTestApp(io.Discard)
		called := false
		app.Command("kick").Action = func(ctx *cli.Context) error {
			called = true
			if !reflect.DeepEqual(ctx.Args().Slice(), tc.want) {
				t.Fatalf("SID list changed: %v", ctx.Args().Slice())
			}
			return nil
		}
		if err := app.Run(append([]string{"ipgw"}, tc.args...)); err != nil || !called {
			t.Fatalf("valid SID list rejected: %v, called=%v", err, called)
		}
	}
}
