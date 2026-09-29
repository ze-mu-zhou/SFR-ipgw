//go:build !windows

package credential

import (
	"errors"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestKeyringSaveLoadDelete(t *testing.T) {
	keyring.MockInit()
	ref, err := NewReference("ipgw-test-user")
	if err != nil {
		t.Fatal(err)
	}
	const password = "P&+空=word"
	if err = Save(ref, "ipgw-test-user", password); err != nil {
		t.Fatal(err)
	}
	got, err := Load(ref)
	if err != nil || got != password {
		t.Fatalf("loaded %q, %v; want %q", got, err, password)
	}
	if err = Delete(ref); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(ref); err == nil {
		t.Fatal("load succeeded after delete")
	}
	// Delete 幂等：条目不存在时也成功
	if err = Delete(ref); err != nil {
		t.Fatal(err)
	}
}

func TestKeyringUnavailableExplainsFallback(t *testing.T) {
	keyring.MockInitWithError(keyring.ErrUnsupportedPlatform)
	t.Cleanup(keyring.MockInit)
	err := Save("ref", "user", "password")
	if err == nil || !strings.Contains(err.Error(), "--no-store-password") {
		t.Fatalf("error=%v", err)
	}
	keyring.MockInitWithError(errors.New("dbus: no session bus"))
	if err = Delete("ref"); err == nil || !strings.Contains(err.Error(), "Secret Service") {
		t.Fatalf("error=%v", err)
	}
}
