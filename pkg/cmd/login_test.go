package cmd

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ze-mu-zhou/SFR-ipgw/pkg/console"
)

func TestDefaultAndExplicitLoginResolveCredentialsConsistently(t *testing.T) {
	// A regular file makes password input deterministically non-interactive.
	stdin, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	transport := &argumentTestTransport{}
	oldStdin, oldTransport := os.Stdin, http.DefaultTransport
	oldOut, oldErr := console.Stdout, console.Stderr
	var output bytes.Buffer
	os.Stdin, http.DefaultTransport = stdin, transport
	console.Stdout, console.Stderr = &output, &output
	t.Cleanup(func() {
		os.Stdin, http.DefaultTransport = oldStdin, oldTransport
		console.Stdout, console.Stderr = oldOut, oldErr
		stdin.Close()
	})

	for _, tc := range []struct {
		name, config, wantError, selected string
		requests                          int
	}{
		{"username only", `{"default_account":"review","accounts":[{"username":"review"}]}`, "请在交互终端运行以隐藏输入密码", "", 0},
		{"first account fallback", `{"accounts":[{"username":"review"}]}`, "请在交互终端运行以隐藏输入密码", "", 0},
		{"saved default", `{"default_account":"saved","accounts":[{"username":"other"},{"username":"saved","credential_ref":"ipgw/account/test"}]}`, "unexpected network request", "saved", 1},
		{"saved first account", `{"accounts":[{"username":"saved","credential_ref":"ipgw/account/test"}]}`, "unexpected network request", "saved", 1},
		{"no accounts", `{"accounts":[]}`, "没有默认账号", "", 0},
		{"missing config", "", "没有默认账号", "", 0},
		{"invalid config", `{broken`, "配置无效", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if tc.config != "" {
				if err := os.WriteFile(path, []byte(tc.config), 0600); err != nil {
					t.Fatal(err)
				}
			}
			var defaultError string
			for _, explicit := range []bool{false, true} {
				output.Reset()
				transport.calls = 0
				args := []string{"ipgw", "--config", path}
				if explicit {
					args = append(args, "login")
				}
				err := argumentTestApp(io.Discard).Run(args)
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("explicit=%v: expected %q, got %v", explicit, tc.wantError, err)
				}
				if transport.calls != tc.requests {
					t.Fatalf("explicit=%v: requests=%d, want %d", explicit, transport.calls, tc.requests)
				}
				if !explicit {
					defaultError = err.Error()
					if tc.selected != "" && !strings.Contains(output.String(), "使用账号 '"+tc.selected+"'") {
						t.Fatalf("wrong default account: %s", output.String())
					}
				} else if err.Error() != defaultError {
					t.Fatalf("entry points differ: default=%q explicit=%q", defaultError, err.Error())
				}
			}
			data, err := os.ReadFile(path)
			if tc.config == "" {
				if !os.IsNotExist(err) {
					t.Fatalf("login created a config file: %v", err)
				}
			} else if err != nil || string(data) != tc.config {
				t.Fatalf("login changed configuration: %v", err)
			}
		})
	}
}
