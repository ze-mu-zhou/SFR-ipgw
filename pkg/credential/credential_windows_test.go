//go:build windows
// +build windows

package credential

import "testing"

func TestSaveLoadDelete(t *testing.T) {
	ref, e := NewReference("ipgw-test-user")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { Delete(ref) })
	const password = "P&+空=word"
	if e = Save(ref, "ipgw-test-user", password); e != nil {
		t.Fatal(e)
	}
	got, e := Load(ref)
	if e != nil {
		t.Fatal(e)
	}
	if got != password {
		t.Fatalf("loaded %q, want %q", got, password)
	}
	if e = Delete(ref); e != nil {
		t.Fatal(e)
	}
	if _, e = Load(ref); e == nil {
		t.Fatal("load succeeded after delete")
	}
	// Delete 幂等：条目不存在时也成功
	if e = Delete(ref); e != nil {
		t.Fatal(e)
	}
}
