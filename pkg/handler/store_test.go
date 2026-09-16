package handler

import (
	"fmt"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/model"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/utils"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingDoesNotCreateConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	s, e := NewStoreHandler(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Load(); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("read created configuration")
	}
}
func TestStoreRejectsCorruptConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	os.WriteFile(path, []byte(`{"accounts":`), 0600)
	s, _ := NewStoreHandler(path)
	if e := s.Load(); e == nil {
		t.Fatal("truncated JSON accepted")
	}
}
func TestPersistOnlySerializableCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	s, _ := NewStoreHandler(path)
	s.Config = &model.Config{Accounts: []*model.Account{{Username: "test", Password: "sensitive", Secret: "secret", Cookie: "cookie", CredentialRef: "vault-ref"}}}
	if e := s.Persist(); e != nil {
		t.Fatal(e)
	}
	s2, _ := NewStoreHandler(path)
	if e := s2.Load(); e != nil {
		t.Fatal(e)
	}
	a := s2.Config.Accounts[0]
	if a.Password != "" || a.Cookie != "" || a.Secret != "" || a.CredentialRef != "vault-ref" {
		t.Fatal("invalid persisted account")
	}
}
func TestMigrationDecodeFailurePreservesConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	s, _ := NewStoreHandler(path)
	s.Config = &model.Config{Accounts: []*model.Account{{Username: "test", EncryptedPassword: "invalid"}}}
	if e := s.Persist(); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(path)
	if e := s.MigrateCredentials(""); e == nil {
		t.Fatal("invalid password migrated")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) || s.Config.Accounts[0].EncryptedPassword != "invalid" {
		t.Fatal("legacy config changed on failure")
	}
}

func TestMigrationCommitAndPersistFailure(t *testing.T) {
	originalSave, originalLoad := saveCredential, loadCredential
	defer func() { saveCredential, loadCredential = originalSave, originalLoad }()
	vault := map[string]string{}
	saveCredential = func(ref, user, password string) error {
		if _, exists := vault[ref]; exists {
			t.Fatal("credential overwritten")
		}
		vault[ref] = password
		return nil
	}
	loadCredential = func(ref string) (string, error) { return vault[ref], nil }
	encrypted, e := utils.Encrypt([]byte("dummy-password"), nil)
	if e != nil {
		t.Fatal(e)
	}
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint("persist failure ", fail), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config")
			s, _ := NewStoreHandler(path)
			s.Config = &model.Config{Accounts: []*model.Account{{Username: "dummy-user", EncryptedPassword: encrypted}}}
			if fail {
				s.Path = filepath.Join(path, "missing", "config")
			}
			e := s.MigrateCredentials("")
			if (e != nil) != fail {
				t.Fatalf("error=%v", e)
			}
			a := s.Config.Accounts[0]
			if fail {
				if a.EncryptedPassword != encrypted || a.CredentialRef != "" {
					t.Fatal("in-memory legacy state changed on failed commit")
				}
			} else {
				if a.EncryptedPassword != "" || a.CredentialRef == "" {
					t.Fatal("migration did not commit")
				}
				s2, _ := NewStoreHandler(path)
				if e = s2.Load(); e != nil {
					t.Fatal(e)
				}
				if s2.Config.Accounts[0].EncryptedPassword != "" || s2.Config.Accounts[0].CredentialRef != a.CredentialRef {
					t.Fatal("migration disk mismatch")
				}
			}
		})
	}
}
