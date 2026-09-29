package handler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/ze-mu-zhou/SFR-ipgw"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/console"
)

type updateTransport func(*http.Request) (*http.Response, error)

func (f updateTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testUpdateHandler(rt http.RoundTripper) *UpdateHandler {
	client := &http.Client{Transport: rt}
	updater := NewUpdateHandler()
	updater.apiClient, updater.downloadClient = client, client
	return updater
}

type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error { b.closed = true; return nil }

func releaseBuild(t *testing.T) {
	t.Helper()
	version, kind, repo, enabled := ipgw.Version, ipgw.BuildKind, ipgw.ReleaseRepo, ipgw.UpdateEnabled
	t.Cleanup(func() {
		ipgw.Version, ipgw.BuildKind, ipgw.ReleaseRepo, ipgw.UpdateEnabled = version, kind, repo, enabled
	})
	ipgw.Version = "v1.0.0"
	ipgw.BuildKind = "release"
	ipgw.ReleaseRepo = "test/ipgw"
	ipgw.UpdateEnabled = "true"
}

func TestLocalUpdateDoesNotContactNetwork(t *testing.T) {
	old := ipgw.BuildKind
	ipgw.BuildKind = "local"
	defer func() { ipgw.BuildKind = old }()
	updater := testUpdateHandler(updateTransport(func(*http.Request) (*http.Response, error) { t.Fatal("local build contacted network"); return nil, nil }))
	if _, err := updater.CheckLatestVersion(); err == nil {
		t.Fatal("local update allowed")
	}
	if err := updater.Update(); err == nil {
		t.Fatal("local update allowed")
	}
}

func TestDownloadStatusAndUnknownLength(t *testing.T) {
	releaseBuild(t)
	for _, status := range []int{200, 404, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader("zip bytes")}
			updater := testUpdateHandler(updateTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Body: body, ContentLength: -1}, nil
			}))
			path, err := updater.download("https://github.com/test/ipgw/releases/download/v1.1.0/ipgw.zip")
			if path != "" {
				defer os.Remove(path)
			}
			if (err == nil) != (status == 200) {
				t.Fatalf("status=%d error=%v", status, err)
			}
			if !body.closed {
				t.Fatal("body leaked")
			}
			if err == nil {
				data, err := os.ReadFile(path)
				if err != nil || string(data) != "zip bytes" {
					t.Fatalf("bad download: %q %v", data, err)
				}
			}
		})
	}
}

func TestDownloadFailureRemovesTemporaryFile(t *testing.T) {
	releaseBuild(t)
	body := &trackedBody{Reader: strings.NewReader("short")}
	updater := testUpdateHandler(updateTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: body, ContentLength: 100}, nil
	}))
	path, err := updater.download("https://github.com/test/ipgw/releases/download/v1/ipgw.zip")
	if err == nil {
		t.Fatal("short download accepted")
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("temporary download left behind: %v", err)
	}
}

func TestDigestValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "asset")
	os.WriteFile(path, []byte("release"), 0600)
	sum := sha256.Sum256([]byte("release"))
	for _, digest := range []string{"", "sha256:bad", "sha256:" + strings.Repeat("0", 64)} {
		if verifyDigest(path, digest) == nil {
			t.Fatalf("accepted %q", digest)
		}
	}
	if err := verifyDigest(path, "sha256:"+hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
}

func TestReplaceExecutableRollback(t *testing.T) {
	dir := t.TempDir()
	current, candidate := filepath.Join(dir, "ipgw"), filepath.Join(dir, "candidate")
	os.WriteFile(current, []byte("old"), 0600)
	os.WriteFile(candidate, []byte("new"), 0600)
	calls := 0
	_, err := replaceExecutable(current, candidate, func(a, b string) error {
		calls++
		if calls == 2 {
			return errors.New("simulated replacement failure")
		}
		return os.Rename(a, b)
	})
	if err == nil {
		t.Fatal("missing replacement error")
	}
	data, err := os.ReadFile(current)
	if err != nil || !bytes.Equal(data, []byte("old")) {
		t.Fatalf("original not restored: %q %v", data, err)
	}
}

func TestReplaceExecutableRetainsBackup(t *testing.T) {
	dir := t.TempDir()
	current, candidate := filepath.Join(dir, "ipgw"), filepath.Join(dir, "candidate")
	os.WriteFile(current, []byte("old"), 0600)
	os.WriteFile(candidate, []byte("new"), 0600)
	backup, err := replaceExecutable(current, candidate, os.Rename)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{backup: "old", current: "new"} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Fatalf("bad file %s: %q %v", path, data, err)
		}
	}
}

func TestInvalidExecutableRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fake")
	os.WriteFile(path, []byte("not an executable"), 0600)
	if validateExecutable(path) == nil {
		t.Fatal("accepted invalid executable")
	}
}

func TestReleaseAPIRejectsFailureAndMalformedVersion(t *testing.T) {
	releaseBuild(t)
	for _, tc := range []struct {
		status int
		body   string
	}{{500, "{}"}, {200, `{"tag_name":"garbage"}`}, {200, `{"tag_name":"v1.2.0","prerelease":true}`}} {
		updater := testUpdateHandler(updateTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
		}))
		if _, err := updater.CheckLatestVersion(); err == nil {
			t.Fatalf("accepted invalid response: %+v", tc)
		}
	}
}

func TestDownloadProgressIsThrottled(t *testing.T) {
	var out strings.Builder
	old := console.Stdout
	console.Stdout = &out
	defer func() { console.Stdout = old }()
	data := strings.Repeat("x", 10000)
	progress := &downloader{Reader: iotest.OneByteReader(strings.NewReader(data)), total: int64(len(data))}
	if _, err := io.Copy(io.Discard, progress); err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(out.String(), "\r"); lines > 10 {
		t.Fatalf("printed progress %d times", lines)
	}
	if !strings.HasSuffix(out.String(), "100.00%") {
		t.Fatalf("final progress missing: %q", out.String())
	}
}
