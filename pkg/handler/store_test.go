package handler

import (
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/model"
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
	s.Config = &model.Config{Accounts: []*model.Account{{Username: "test", Password: "sensitive", Cookie: "cookie", CredentialRef: "vault-ref"}}}
	if e := s.Persist(); e != nil {
		t.Fatal(e)
	}
	s2, _ := NewStoreHandler(path)
	if e := s2.Load(); e != nil {
		t.Fatal(e)
	}
	a := s2.Config.Accounts[0]
	if a.Password != "" || a.Cookie != "" || a.CredentialRef != "vault-ref" {
		t.Fatal("invalid persisted account")
	}
}
