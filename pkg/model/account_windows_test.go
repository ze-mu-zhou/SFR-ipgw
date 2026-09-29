//go:build windows

package model

import (
	"testing"

	"github.com/ze-mu-zhou/SFR-ipgw/pkg/credential"
)

func TestAccountChangesPreserveOldCredentialBeforeCommit(t *testing.T) {
	account := &Account{Username: "ipgw-test-uncommitted-account"}
	if err := account.SetPassword("test-only-old-password"); err != nil {
		t.Fatal(err)
	}
	oldRef := account.CredentialRef
	t.Cleanup(func() { credential.Delete(oldRef) })
	if err := account.SetPassword("test-only-new-password"); err != nil {
		t.Fatal(err)
	}
	newRef := account.CredentialRef
	t.Cleanup(func() { credential.Delete(newRef) })
	if oldRef == newRef {
		t.Fatal("password update overwrote the committed credential")
	}
	if password, err := credential.Load(oldRef); err != nil || password != "test-only-old-password" {
		t.Fatal("password update removed the old credential before commit")
	}
	config := &Config{DefaultAccount: account.Username, Accounts: []*Account{account}}
	if err := config.DeleteAccount(account.Username); err != nil {
		t.Fatal(err)
	}
	if password, err := credential.Load(newRef); err != nil || password != "test-only-new-password" {
		t.Fatal("account deletion removed its credential before commit")
	}
}
