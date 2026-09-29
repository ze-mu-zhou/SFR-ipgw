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
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	header := &zip.FileHeader{Name: name}
	header.SetMode(mode)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	entry.Write([]byte("data"))
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	file.Close()
	return path
}

func TestUnzipRejectsUnsafeEntries(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode os.FileMode
	}{{"../outside", 0600}, {"/absolute", 0600}, {"a/../../escape", 0600}, {"C:/outside", 0600}, {"..\\escape", 0600}, {"file:stream", 0600}, {"link", os.ModeSymlink | 0700}} {
		t.Run(tc.name, func(t *testing.T) {
			if err := Unzip(testZip(t, tc.name, tc.mode), filepath.Join(t.TempDir(), "dest")); err == nil {
				t.Fatal("unsafe archive accepted")
			}
		})
	}
}

func TestUnzipExtractsButNeverOverwrites(t *testing.T) {
	dest := t.TempDir()
	archive := testZip(t, "sub/ipgw", 0755)
	if err := Unzip(archive, dest); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dest, "sub", "ipgw")
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "data" {
		t.Fatalf("bad extraction %q %v", data, err)
	}
	os.WriteFile(path, []byte("existing"), 0600)
	if err = Unzip(archive, dest); err == nil {
		t.Fatal("overwrote existing file")
	}
	data, _ = os.ReadFile(path)
	if string(data) != "existing" {
		t.Fatal("existing file changed")
	}
}

func TestSemverNumericPrereleaseAndInvalidInput(t *testing.T) {
	for _, version := range []string{"1.2.3.4", "1.a.0", "01.2.3", "1.2.3-01", "1.2.3-", "1.2.3+"} {
		if ParseVersion(version) != nil {
			t.Fatalf("invalid version accepted: %s", version)
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

func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
}

// macOS 的临时目录位于 /var -> /private/var 之下，目标目录的上级链接不应被拒绝。
func TestUnzipAllowsSymlinkedDestinationParent(t *testing.T) {
	base := t.TempDir()
	actual := filepath.Join(base, "actual")
	if err := os.Mkdir(actual, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	symlinkOrSkip(t, actual, link)
	if err := Unzip(testZip(t, "sub/ipgw", 0755), filepath.Join(link, "dest")); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(actual, "dest", "sub", "ipgw")); err != nil || string(data) != "data" {
		t.Fatalf("bad extraction %q %v", data, err)
	}
}

func TestUnzipRejectsSymlinkInsideDestination(t *testing.T) {
	dest, outside := t.TempDir(), t.TempDir()
	symlinkOrSkip(t, outside, filepath.Join(dest, "sub"))
	if err := Unzip(testZip(t, "sub/ipgw", 0755), dest); err == nil {
		t.Fatal("extracted through a symlink inside the destination")
	}
	if _, err := os.Stat(filepath.Join(outside, "ipgw")); !os.IsNotExist(err) {
		t.Fatalf("wrote outside destination: %v", err)
	}
}
