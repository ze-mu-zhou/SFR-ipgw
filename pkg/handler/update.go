package handler

import (
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/ze-mu-zhou/SFR-ipgw"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/console"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/utils"
)

type releaseAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"`
}
type releaseInfo struct {
	Version    string         `json:"tag_name"`
	Draft      bool           `json:"draft"`
	Prerelease bool           `json:"prerelease"`
	Assets     []releaseAsset `json:"assets"`
}
type UpdateHandler struct {
	client  *http.Client
	release *releaseInfo
}
type downloader struct {
	io.Reader
	total, current int64
}

func (d *downloader) Read(p []byte) (int, error) {
	n, e := d.Reader.Read(p)
	d.current += int64(n)
	if d.total > 0 {
		console.InfoF("\rdownloading %.2f%%", float64(d.current)/float64(d.total)*100)
	} else {
		console.InfoF("\rdownloading %d bytes", d.current)
	}
	return n, e
}
func NewUpdateHandler() *UpdateHandler {
	return &UpdateHandler{client: &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return fmt.Errorf("refusing non-HTTPS update redirect")
		}
		if len(via) >= 10 {
			return fmt.Errorf("too many update redirects")
		}
		return nil
	}}}
}

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func updateAllowed() error {
	if ipgw.BuildKind != "release" || ipgw.UpdateEnabled != "true" || !repositoryPattern.MatchString(ipgw.ReleaseRepo) || utils.ParseVersion(ipgw.Version) == nil {
		return fmt.Errorf("self-update is disabled for this local/custom build; rebuild manually, or use a release explicitly configured with a compatible release repository")
	}
	return nil
}
func (u *UpdateHandler) CheckLatestVersion() (bool, error) {
	if e := updateAllowed(); e != nil {
		return false, e
	}
	u.release = nil
	resp, e := u.client.Get("https://api.github.com/repos/" + ipgw.ReleaseRepo + "/releases/latest")
	if e != nil {
		return false, fmt.Errorf("check latest version: %w", e)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("release API returned HTTP %d", resp.StatusCode)
	}
	var release releaseInfo
	if e = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&release); e != nil {
		return false, fmt.Errorf("decode release: %w", e)
	}
	if release.Draft || release.Prerelease || utils.ParseVersion(release.Version) == nil {
		return false, fmt.Errorf("release API returned an invalid stable version")
	}
	u.release = &release
	return utils.CompareVersion(utils.ParseVersion(release.Version), utils.ParseVersion(ipgw.Version)), nil
}
func (u *UpdateHandler) download(rawURL string) (path string, err error) {
	parsed, e := url.Parse(rawURL)
	if e != nil || parsed.Scheme != "https" || parsed.Host != "github.com" || parsed.User != nil || !strings.HasPrefix(parsed.Path, "/"+ipgw.ReleaseRepo+"/releases/download/") {
		return "", fmt.Errorf("download URL is outside the configured release repository")
	}
	resp, e := u.client.Get(rawURL)
	if e != nil {
		return "", e
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > 128<<20 {
		return "", fmt.Errorf("download exceeds 128 MiB limit")
	}
	f, e := os.CreateTemp("", "ipgw.release.*")
	if e != nil {
		return "", e
	}
	path = f.Name()
	defer func() {
		if err != nil {
			os.Remove(path)
		}
	}()
	n, copyErr := io.Copy(f, &downloader{Reader: io.LimitReader(resp.Body, (128<<20)+1), total: resp.ContentLength})
	console.InfoL()
	closeErr := f.Close()
	if copyErr != nil {
		return path, copyErr
	}
	if closeErr != nil {
		return path, closeErr
	}
	if n > 128<<20 {
		return path, fmt.Errorf("download exceeds 128 MiB limit")
	}
	if resp.ContentLength >= 0 && n != resp.ContentLength {
		return path, fmt.Errorf("incomplete download")
	}
	return path, nil
}
func verifyDigest(path, digest string) error {
	if !strings.HasPrefix(digest, "sha256:") {
		return fmt.Errorf("release asset is missing a SHA-256 digest")
	}
	expected, e := hex.DecodeString(strings.TrimPrefix(digest, "sha256:"))
	if e != nil || len(expected) != sha256.Size {
		return fmt.Errorf("release asset has an invalid SHA-256 digest")
	}
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return e
	}
	if hex.EncodeToString(h.Sum(nil)) != hex.EncodeToString(expected) {
		return fmt.Errorf("download SHA-256 does not match release metadata")
	}
	return nil
}
func validateExecutable(path string) error {
	info, e := buildinfo.ReadFile(path)
	if e != nil {
		return fmt.Errorf("invalid Go executable: %w", e)
	}
	if info.Main.Path != "github.com/ze-mu-zhou/SFR-ipgw" || (info.Path != "command-line-arguments" && info.Path != "github.com/ze-mu-zhou/SFR-ipgw/cmd/ipgw") {
		return fmt.Errorf("release contains an unexpected application %q", info.Path)
	}
	settings := map[string]string{}
	for _, s := range info.Settings {
		settings[s.Key] = s.Value
	}
	if settings["GOOS"] != runtime.GOOS || settings["GOARCH"] != runtime.GOARCH {
		return fmt.Errorf("release executable platform does not match %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return nil
}

// replaceExecutable keeps the old executable as a recovery copy. On Windows it
// can remain locked until this process exits, so it is not deleted automatically.
func replaceExecutable(current, candidate string, rename func(string, string) error) (string, error) {
	temp, e := os.CreateTemp(filepath.Dir(current), ".ipgw-backup-*")
	if e != nil {
		return "", e
	}
	backup := temp.Name()
	if e = temp.Close(); e != nil {
		return "", e
	}
	if e = os.Remove(backup); e != nil {
		return "", e
	}
	if e = rename(current, backup); e != nil {
		return "", fmt.Errorf("back up executable: %w", e)
	}
	if e = rename(candidate, current); e != nil {
		if rollbackErr := rename(backup, current); rollbackErr != nil {
			return backup, fmt.Errorf("install failed: %v; rollback failed: %v; recover executable from %s", e, rollbackErr, backup)
		}
		return "", fmt.Errorf("install failed; original executable restored: %w", e)
	}
	return backup, nil
}
func (u *UpdateHandler) Update() error {
	if e := updateAllowed(); e != nil {
		return e
	}
	if u.release == nil {
		newer, e := u.CheckLatestVersion()
		if e != nil {
			return e
		}
		if !newer {
			return nil
		}
	}
	if !utils.CompareVersion(utils.ParseVersion(u.release.Version), utils.ParseVersion(ipgw.Version)) {
		return nil
	}
	name := fmt.Sprintf("ipgw-%s-%s.zip", runtime.GOOS, runtime.GOARCH)
	var asset *releaseAsset
	for i := range u.release.Assets {
		if u.release.Assets[i].Name == name {
			asset = &u.release.Assets[i]
			break
		}
	}
	if asset == nil {
		return fmt.Errorf("release has no asset %s", name)
	}
	if asset.Digest == "" {
		return fmt.Errorf("release asset has no SHA-256 digest; manual update required")
	}
	downloaded, e := u.download(asset.URL)
	if e != nil {
		return e
	}
	defer os.Remove(downloaded)
	if e = verifyDigest(downloaded, asset.Digest); e != nil {
		return e
	}
	current, dir, e := utils.GetExecutablePathAndDir()
	if e != nil {
		return e
	}
	stage, e := os.MkdirTemp(dir, ".ipgw-update-*")
	if e != nil {
		return e
	}
	defer os.RemoveAll(stage)
	if e = utils.Unzip(downloaded, stage); e != nil {
		return e
	}
	executable := "ipgw"
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	candidate := filepath.Join(stage, executable)
	if e = validateExecutable(candidate); e != nil {
		return e
	}
	mode := os.FileMode(0755)
	if st, err := os.Stat(current); err == nil {
		mode = st.Mode().Perm()
	}
	if e = os.Chmod(candidate, mode); e != nil {
		return e
	}
	backup, e := replaceExecutable(current, candidate, os.Rename)
	if e != nil {
		return e
	}
	console.InfoF("previous executable retained at %s\n", backup)
	return nil
}
