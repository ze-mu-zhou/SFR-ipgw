//go:build windows

package model

import (
	"testing"

	"github.com/ze-mu-zhou/SFR-ipgw/pkg/credential"
)

func TestAccountChangesPreserveOldCredentialBeforeCommit(t *testing.T) {
	a := &Account{Username: "ipgw-test-uncommitted-account"}
	if err := a.SetPassword("test-only-old-password", nil); err != nil {
		t.Fatal(err)
	}
	oldRef := a.CredentialRef
	t.Cleanup(func() { credential.Delete(oldRef) })
	if err := a.SetPassword("test-only-new-password", nil); err != nil {
		t.Fatal(err)
	}
	newRef := a.CredentialRef
	t.Cleanup(func() { credential.Delete(newRef) })
	if oldRef == newRef {
		t.Fatal("password update overwrote the committed credential")
	}
	if password, err := credential.Load(oldRef); err != nil || password != "test-only-old-password" {
		t.Fatal("password update removed the old credential before commit")
	}
	config := &Config{DefaultAccount: a.Username, Accounts: []*Account{a}}
	if err := config.DelAccount(a.Username); err != nil {
		t.Fatal(err)
	}
	if password, err := credential.Load(newRef); err != nil || password != "test-only-new-password" {
		t.Fatal("account deletion removed its credential before commit")
	}
}
