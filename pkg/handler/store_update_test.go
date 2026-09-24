package handler

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ze-mu-zhou/SFR-ipgw/pkg/model"
)

func TestUpdateConfigCredentialLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name, operation                 string
		persistFailure, changeFailure   bool
		cleanupFailure, sharedReference bool
	}{
		{name: "replace", operation: "set"},
		{name: "delete", operation: "del"},
		{name: "add", operation: "add"},
		{name: "replace save failure", operation: "set", persistFailure: true},
		{name: "delete save failure", operation: "del", persistFailure: true},
		{name: "add save failure", operation: "add", persistFailure: true},
		{name: "change failure", operation: "set", changeFailure: true},
		{name: "replace cleanup warning", operation: "set", cleanupFailure: true},
		{name: "delete cleanup warning", operation: "del", cleanupFailure: true},
		{name: "rollback cleanup error", operation: "set", persistFailure: true, cleanupFailure: true},
		{name: "shared credential survives deletion", operation: "del", sharedReference: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			store, _ := NewStoreHandler(path)
			store.Config = &model.Config{DefaultAccount: "test", Accounts: []*model.Account{{Username: "test", CredentialRef: "old"}}}
			if tc.sharedReference {
				store.Config.Accounts = append(store.Config.Accounts, &model.Account{Username: "other", CredentialRef: "old"})
			}
			if err := store.Persist(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			original := store.Config
			vault := map[string]string{"old": "old-password"}
			cleanupErr := errors.New("simulated vault deletion failure")
			changeErr := errors.New("simulated change failure")
			oldDelete := deleteCredential
			t.Cleanup(func() { deleteCredential = oldDelete })
			deleteCredential = func(ref string) error {
				// At every cleanup call, all references committed to disk must remain usable.
				disk, _ := NewStoreHandler(path)
				if err := disk.Load(); err != nil {
					t.Fatal(err)
				}
				for _, a := range disk.Config.Accounts {
					if a.CredentialRef == ref {
						t.Fatalf("deleted credential still referenced on disk: %s", ref)
					}
				}
				if tc.cleanupFailure {
					return cleanupErr
				}
				delete(vault, ref)
				return nil
			}
			warning, updateErr := store.UpdateConfig(func(config *model.Config) error {
				// Fail the write after the locked reload, not lock acquisition.
				if tc.persistFailure {
					store.Path = filepath.Join(path, "missing", "config.json")
				}
				switch tc.operation {
				case "set":
					vault["new"] = "new-password"
					config.GetAccount("test").CredentialRef = "new"
				case "add":
					vault["new"] = "new-password"
					config.Accounts = append(config.Accounts, &model.Account{Username: "new-user", CredentialRef: "new"})
				case "del":
					if err := config.DelAccount("test"); err != nil {
						return err
					}
				}
				if tc.changeFailure {
					return changeErr
				}
				return nil
			})
			failed := tc.persistFailure || tc.changeFailure
			if (updateErr != nil) != failed || (warning != nil) != (tc.cleanupFailure && !failed) {
				t.Fatalf("unexpected result: warning=%v error=%v", warning, updateErr)
			}
			if tc.changeFailure && !errors.Is(updateErr, changeErr) {
				t.Fatal("lost original change error")
			}
			if tc.cleanupFailure && !errors.Is(errors.Join(warning, updateErr), cleanupErr) {
				t.Fatal("lost cleanup error")
			}
			disk, _ := NewStoreHandler(path)
			if err := disk.Load(); err != nil {
				t.Fatal(err)
			}
			if failed {
				after, err := os.ReadFile(path)
				if err != nil || string(before) != string(after) || store.Config != original {
					t.Fatal("failed change did not preserve disk and in-memory state")
				}
			} else if !reflect.DeepEqual(disk.Config, store.Config) {
				t.Fatal("committed disk and in-memory state differ")
			}
			if original.DefaultAccount != "test" || original.Accounts[0].CredentialRef != "old" {
				t.Fatal("original configuration was mutated")
			}
			wantOld := failed || tc.operation == "add" || tc.cleanupFailure || tc.sharedReference
			wantNew := tc.operation != "del" && (!failed || tc.cleanupFailure)
			if (vault["old"] != "") != wantOld || (vault["new"] != "") != wantNew {
				t.Fatalf("unexpected vault contents: %v", vault)
			}
			for _, a := range disk.Config.Accounts {
				if vault[a.CredentialRef] == "" {
					t.Fatal("persisted account has no usable credential")
				}
			}
		})
	}
}
