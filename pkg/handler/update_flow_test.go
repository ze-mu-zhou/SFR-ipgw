package handler

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func updateArchive(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var data bytes.Buffer
	z := zip.NewWriter(&data)
	entry, err := z.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func updateExecutableName() string {
	if runtime.GOOS == "windows" {
		return "ipgw.exe"
	}
	return "ipgw"
}

func flowUpdater(t *testing.T, archive []byte, digest, version string) (*UpdateHandler, string, *[]string) {
	t.Helper()
	dir := t.TempDir()
	current := filepath.Join(dir, updateExecutableName())
	if err := os.WriteFile(current, []byte("old executable"), 0755); err != nil {
		t.Fatal(err)
	}
	assetName := fmt.Sprintf("ipgw-%s-%s.zip", runtime.GOOS, runtime.GOARCH)
	assetURL := "https://github.com/test/ipgw/releases/download/" + version + "/" + assetName
	metadata, err := json.Marshal(releaseInfo{Version: version, Assets: []releaseAsset{{Name: assetName, URL: assetURL, Digest: digest}}})
	if err != nil {
		t.Fatal(err)
	}
	var requests []string
	u := testUpdateHandler(updateTransport(func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r.URL.String())
		var data []byte
		switch r.URL.String() {
		case "https://api.github.com/repos/test/ipgw/releases/latest":
			data = metadata
		case assetURL:
			data = archive
		default:
			t.Fatalf("unexpected update request: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data)), Request: r}, nil
	}))
	// The full Update pipeline installs into a disposable directory, never the
	// currently running test executable or a user's installed program.
	u.executablePath = func() (string, string, error) { return current, dir, nil }
	return u, current, &requests
}

func archiveDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func TestUpdateFlowRejectsInvalidAssetsWithoutReplacingCurrent(t *testing.T) {
	releaseBuild(t)
	badExecutable := updateArchive(t, updateExecutableName(), []byte("not a Go executable"))
	wrongEntry := updateArchive(t, "unexpected.txt", []byte("not the application"))
	for _, tc := range []struct {
		name, digest, wantError string
		archive                 []byte
	}{
		{"missing digest", "", "没有 SHA-256", badExecutable},
		{"digest mismatch", "sha256:" + strings.Repeat("0", 64), "不匹配", badExecutable},
		{"invalid ZIP", archiveDigest([]byte("broken")), "zip", []byte("broken")},
		{"invalid executable", archiveDigest(badExecutable), "无效的 Go 可执行文件", badExecutable},
		{"missing executable", archiveDigest(wrongEntry), "无效的 Go 可执行文件", wrongEntry},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u, current, _ := flowUpdater(t, tc.archive, tc.digest, "v1.1.0")
			if err := u.Update(); err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("expected %q, got %v", tc.wantError, err)
			}
			data, err := os.ReadFile(current)
			if err != nil || string(data) != "old executable" {
				t.Fatalf("current executable changed: %q %v", data, err)
			}
			entries, err := os.ReadDir(filepath.Dir(current))
			if err != nil || len(entries) != 1 {
				t.Fatalf("failed update left staging or backup files: %v %v", entries, err)
			}
		})
	}
}

func TestUpdateFlowDoesNotDownloadOlderOrEqualVersion(t *testing.T) {
	releaseBuild(t)
	for _, version := range []string{"v0.9.0", "v1.0.0"} {
		u, _, requests := flowUpdater(t, nil, "", version)
		if err := u.Update(); err != nil || len(*requests) != 1 {
			t.Fatalf("version=%s error=%v requests=%v", version, err, *requests)
		}
	}
}

func TestUpdateFlowInstallsValidatedExecutableAndRetainsBackup(t *testing.T) {
	releaseBuild(t)
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("Go compiler needed to build a real release candidate")
	}
	candidate := filepath.Join(t.TempDir(), updateExecutableName())
	build := exec.Command(goTool, "build", "-o", candidate, "./cmd/ipgw")
	build.Dir = filepath.Join("..", "..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build candidate: %v\n%s", err, output)
	}
	binary, err := os.ReadFile(candidate)
	if err != nil {
		t.Fatal(err)
	}
	archive := updateArchive(t, updateExecutableName(), binary)
	u, current, requests := flowUpdater(t, archive, archiveDigest(archive), "v1.1.0")
	if err := u.Update(); err != nil {
		t.Fatal(err)
	}
	if len(*requests) != 2 {
		t.Fatalf("expected one metadata request and one pinned asset download: %v", *requests)
	}
	installed, err := os.ReadFile(current)
	if err != nil || !bytes.Equal(installed, binary) {
		t.Fatalf("candidate not installed: %v", err)
	}
	backups, err := filepath.Glob(filepath.Join(filepath.Dir(current), ".ipgw-backup-*"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("missing recovery backup: %v %v", backups, err)
	}
	old, err := os.ReadFile(backups[0])
	if err != nil || string(old) != "old executable" {
		t.Fatalf("invalid backup: %q %v", old, err)
	}
	stages, err := filepath.Glob(filepath.Join(filepath.Dir(current), ".ipgw-update-*"))
	if err != nil || len(stages) != 0 {
		t.Fatalf("staging directories leaked: %v %v", stages, err)
	}
}
