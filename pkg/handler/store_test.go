package handler

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ze-mu-zhou/SFR-ipgw/pkg/model"
)

// persist writes s.Config as-is under the lock, to seed test fixtures. It does
// not reload first, so production code must use UpdateConfig instead.
func (s *StoreHandler) persist() error {
	if s.Config == nil {
		return errors.New("未加载配置")
	}
	lock, err := lockConfig(s.Path)
	if err != nil {
		return err
	}
	defer lock.Close()
	return s.persistLocked()
}

func TestLoadMissingDoesNotCreateConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	store, err := NewStoreHandler(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Load(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("read created configuration")
	}
}

func TestStoreRejectsCorruptConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	os.WriteFile(path, []byte(`{"accounts":`), 0600)
	store, _ := NewStoreHandler(path)
	if err := store.Load(); err == nil {
		t.Fatal("truncated JSON accepted")
	}
}

func TestPersistOnlySerializableCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	store, _ := NewStoreHandler(path)
	store.Config = &model.Config{Accounts: []*model.Account{{Username: "test", Password: "sensitive", CredentialRef: "vault-ref"}}}
	if err := store.persist(); err != nil {
		t.Fatal(err)
	}
	reloaded, _ := NewStoreHandler(path)
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	account := reloaded.Config.Accounts[0]
	if account.Password != "" || account.CredentialRef != "vault-ref" {
		t.Fatal("invalid persisted account")
	}
}
