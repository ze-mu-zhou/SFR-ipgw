//go:build windows

package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v2"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/handler"
)

func TestDeleteAccountCleanupFailureIsWarningAfterCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	// An invalid reference reliably fails validation without touching the real vault.
	if err := os.WriteFile(path, []byte(`{"default_account":"test","accounts":[{"username":"test","credential_ref":"invalid\u0000reference"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	store, _ := handler.NewStoreHandler(path)
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
