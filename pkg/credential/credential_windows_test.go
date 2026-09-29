//go:build windows

package credential

import "testing"

func TestSaveLoadDelete(t *testing.T) {
	ref, err := NewReference("ipgw-test-user")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { Delete(ref) })
	const password = "P&+空=word"
	if err = Save(ref, "ipgw-test-user", password); err != nil {
		t.Fatal(err)
	}
	got, err := Load(ref)
	if err != nil {
		t.Fatal(err)
	}
	if got != password {
		t.Fatalf("loaded %q, want %q", got, password)
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
