//go:build windows

package cmd

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v2"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/handler"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/model"
)

func TestDeleteAccountCleanupFailureIsWarningAfterCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store, _ := handler.NewStoreHandler(path)
	// An invalid reference reliably fails validation without touching the real vault.
	store.Config = &model.Config{DefaultAccount: "test", Accounts: []*model.Account{{Username: "test", CredentialRef: "invalid\x00reference"}}}
	if err := store.Persist(); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	app := &cli.App{
		Writer: io.Discard, ErrWriter: &stderr,
		Flags:    []cli.Flag{&cli.StringFlag{Name: "config"}},
		Commands: []*cli.Command{ConfigCommand},
	}
	if err := app.Run([]string{"ipgw", "--config", path, "config", "account", "del", "-u", "test"}); err != nil {
		t.Fatalf("committed deletion returned a failure: %v", err)
	}
	if !strings.Contains(stderr.String(), "配置已保存，但清理旧凭据失败") {
		t.Fatalf("missing cleanup warning: %s", stderr.String())
	}
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if len(store.Config.Accounts) != 0 || store.Config.DefaultAccount != "" {
		t.Fatal("account deletion was not persisted")
	}
}
