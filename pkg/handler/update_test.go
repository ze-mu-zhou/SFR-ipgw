package handler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/ze-mu-zhou/SFR-ipgw"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type updateTransport func(*http.Request) (*http.Response, error)

func (f updateTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testUpdateHandler(rt http.RoundTripper) *UpdateHandler {
	c := &http.Client{Transport: rt}
	u := NewUpdateHandler()
	u.apiClient, u.downloadClient = c, c
	return u
}

type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error { b.closed = true; return nil }
func releaseBuild(t *testing.T) {
	t.Helper()
	v, k, r, e := ipgw.Version, ipgw.BuildKind, ipgw.ReleaseRepo, ipgw.UpdateEnabled
	t.Cleanup(func() { ipgw.Version, ipgw.BuildKind, ipgw.ReleaseRepo, ipgw.UpdateEnabled = v, k, r, e })
	ipgw.Version = "v1.0.0"
	ipgw.BuildKind = "release"
	ipgw.ReleaseRepo = "test/ipgw"
	ipgw.UpdateEnabled = "true"
}
func TestLocalUpdateDoesNotContactNetwork(t *testing.T) {
	old := ipgw.BuildKind
	ipgw.BuildKind = "local"
	defer func() { ipgw.BuildKind = old }()
	u := testUpdateHandler(updateTransport(func(*http.Request) (*http.Response, error) { t.Fatal("local build contacted network"); return nil, nil }))
	if _, e := u.CheckLatestVersion(); e == nil {
		t.Fatal("local update allowed")
	}
	if e := u.Update(); e == nil {
		t.Fatal("local update allowed")
	}
}
func TestDownloadStatusAndUnknownLength(t *testing.T) {
	releaseBuild(t)
	for _, status := range []int{200, 404, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader("zip bytes")}
			u := testUpdateHandler(updateTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Body: body, ContentLength: -1}, nil
			}))
			path, e := u.download("https://github.com/test/ipgw/releases/download/v1.1.0/ipgw.zip")
			if path != "" {
				defer os.Remove(path)
			}
			if (e == nil) != (status == 200) {
				t.Fatalf("status=%d error=%v", status, e)
			}
			if !body.closed {
				t.Fatal("body leaked")
			}
			if e == nil {
				data, e := os.ReadFile(path)
				if e != nil || string(data) != "zip bytes" {
					t.Fatalf("bad download: %q %v", data, e)
				}
			}
		})
	}
}
func TestDownloadFailureRemovesTemporaryFile(t *testing.T) {
	releaseBuild(t)
	body := &trackedBody{Reader: strings.NewReader("short")}
	u := testUpdateHandler(updateTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: body, ContentLength: 100}, nil
	}))
	path, e := u.download("https://github.com/test/ipgw/releases/download/v1/ipgw.zip")
	if e == nil {
		t.Fatal("short download accepted")
	}
	if _, e = os.Stat(path); !os.IsNotExist(e) {
		t.Fatalf("temporary download left behind: %v", e)
	}
}
func TestDigestValidation(t *testing.T) {
	p := filepath.Join(t.TempDir(), "asset")
	os.WriteFile(p, []byte("release"), 0600)
	sum := sha256.Sum256([]byte("release"))
	for _, digest := range []string{"", "sha256:bad", "sha256:" + strings.Repeat("0", 64)} {
		if verifyDigest(p, digest) == nil {
			t.Fatalf("accepted %q", digest)
		}
	}
	if e := verifyDigest(p, "sha256:"+hex.EncodeToString(sum[:])); e != nil {
		t.Fatal(e)
	}
}
func TestReplaceExecutableRollback(t *testing.T) {
	dir := t.TempDir()
	current, candidate := filepath.Join(dir, "ipgw"), filepath.Join(dir, "candidate")
	os.WriteFile(current, []byte("old"), 0600)
	os.WriteFile(candidate, []byte("new"), 0600)
	calls := 0
	_, e := replaceExecutable(current, candidate, func(a, b string) error {
		calls++
		if calls == 2 {
			return errors.New("simulated replacement failure")
		}
		return os.Rename(a, b)
	})
	if e == nil {
		t.Fatal("missing replacement error")
	}
	data, e := os.ReadFile(current)
	if e != nil || !bytes.Equal(data, []byte("old")) {
		t.Fatalf("original not restored: %q %v", data, e)
	}
}
func TestReplaceExecutableRetainsBackup(t *testing.T) {
	dir := t.TempDir()
	current, candidate := filepath.Join(dir, "ipgw"), filepath.Join(dir, "candidate")
	os.WriteFile(current, []byte("old"), 0600)
	os.WriteFile(candidate, []byte("new"), 0600)
	backup, e := replaceExecutable(current, candidate, os.Rename)
	if e != nil {
		t.Fatal(e)
	}
	for p, want := range map[string]string{backup: "old", current: "new"} {
		data, e := os.ReadFile(p)
		if e != nil || string(data) != want {
			t.Fatalf("bad file %s: %q %v", p, data, e)
		}
	}
}
func TestInvalidExecutableRejected(t *testing.T) {
	p := filepath.Join(t.TempDir(), "fake")
	os.WriteFile(p, []byte("not an executable"), 0600)
	if validateExecutable(p) == nil {
		t.Fatal("accepted invalid executable")
	}
}
func TestReleaseAPIRejectsFailureAndMalformedVersion(t *testing.T) {
	releaseBuild(t)
	for _, c := range []struct {
		status int
		body   string
	}{{500, "{}"}, {200, `{"tag_name":"garbage"}`}, {200, `{"tag_name":"v1.2.0","prerelease":true}`}} {
		u := testUpdateHandler(updateTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: c.status, Body: io.NopCloser(strings.NewReader(c.body))}, nil
		}))
		if _, e := u.CheckLatestVersion(); e == nil {
			t.Fatalf("accepted invalid response: %+v", c)
		}
	}
}
