package utils

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func testZip(t *testing.T, name string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.zip")
	f, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	z := zip.NewWriter(f)
	h := &zip.FileHeader{Name: name}
	h.SetMode(mode)
	w, e := z.CreateHeader(h)
	if e != nil {
		t.Fatal(e)
	}
	w.Write([]byte("data"))
	if e = z.Close(); e != nil {
		t.Fatal(e)
	}
	f.Close()
	return path
}
func TestUnzipRejectsUnsafeEntries(t *testing.T) {
	for _, c := range []struct {
		name string
		mode os.FileMode
	}{{"../outside", 0600}, {"/absolute", 0600}, {"a/../../escape", 0600}, {"C:/outside", 0600}, {"..\\escape", 0600}, {"file:stream", 0600}, {"link", os.ModeSymlink | 0700}} {
		t.Run(c.name, func(t *testing.T) {
			if e := Unzip(testZip(t, c.name, c.mode), filepath.Join(t.TempDir(), "dest")); e == nil {
				t.Fatal("unsafe archive accepted")
			}
		})
	}
}
func TestUnzipExtractsButNeverOverwrites(t *testing.T) {
	dest := t.TempDir()
	archive := testZip(t, "sub/ipgw", 0755)
	if e := Unzip(archive, dest); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(dest, "sub", "ipgw")
	data, e := os.ReadFile(p)
	if e != nil || string(data) != "data" {
		t.Fatalf("bad extraction %q %v", data, e)
	}
	os.WriteFile(p, []byte("existing"), 0600)
	if e = Unzip(archive, dest); e == nil {
		t.Fatal("overwrote existing file")
	}
	data, _ = os.ReadFile(p)
	if string(data) != "existing" {
		t.Fatal("existing file changed")
	}
}
func TestSemverNumericPrereleaseAndInvalidInput(t *testing.T) {
	for _, s := range []string{"1.2.3.4", "1.a.0", "01.2.3", "1.2.3-01", "1.2.3-", "1.2.3+"} {
		if ParseVersion(s) != nil {
			t.Fatalf("invalid version accepted: %s", s)
		}
	}
	if !CompareVersion(ParseVersion("1.0.0-rc.10"), ParseVersion("1.0.0-rc.9")) {
		t.Fatal("numeric identifiers compared lexically")
	}
	if CompareVersion(ParseVersion("1.0.0+foo"), ParseVersion("1.0.0+bar")) {
		t.Fatal("build metadata changed precedence")
	}
	if ParseVersion("1.0.0-alpha-beta.1+build.5") == nil {
		t.Fatal("valid version rejected")
	}
}
