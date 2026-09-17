package handler

import (
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
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
	apiClient      *http.Client
	downloadClient *http.Client
	release        *releaseInfo
}
type downloader struct {
	io.Reader
	total, current int64
}

func (d *downloader) Read(p []byte) (int, error) {
	n, e := d.Reader.Read(p)
	d.current += int64(n)
	if d.total > 0 {
		console.InfoF("\r已下载 %.2f%%", float64(d.current)/float64(d.total)*100)
	} else {
		console.InfoF("\r已下载 %d 字节", d.current)
	}
	return n, e
}
func httpsOnlyRedirect(req *http.Request, via []*http.Request) error {
	if req.URL.Scheme != "https" {
		return fmt.Errorf("更新重定向到非 HTTPS 地址，已拒绝")
	}
	if len(via) >= 10 {
		return fmt.Errorf("更新重定向次数过多")
	}
	return nil
}

// API 检查使用总超时；下载只限制连接和响应头，响应体读取不设总时长，
// 慢速网络下也能完成大文件下载。
func NewUpdateHandler() *UpdateHandler {
	return &UpdateHandler{
		apiClient: &http.Client{Timeout: 30 * time.Second, CheckRedirect: httpsOnlyRedirect},
		downloadClient: &http.Client{
			CheckRedirect: httpsOnlyRedirect,
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: 30 * time.Second}).DialContext,
				TLSHandshakeTimeout:   30 * time.Second,
				ResponseHeaderTimeout: 60 * time.Second,
			},
		},
	}
}

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func updateAllowed() error {
	if ipgw.BuildKind != "release" || ipgw.UpdateEnabled != "true" || !repositoryPattern.MatchString(ipgw.ReleaseRepo) || utils.ParseVersion(ipgw.Version) == nil {
		return fmt.Errorf("本地/自定义构建已禁用自动更新；请手动重新构建，或使用配置了兼容发布仓库的正式版本")
	}
	return nil
}
func (u *UpdateHandler) CheckLatestVersion() (bool, error) {
	if e := updateAllowed(); e != nil {
		return false, e
	}
	u.release = nil
	resp, e := u.apiClient.Get("https://api.github.com/repos/" + ipgw.ReleaseRepo + "/releases/latest")
	if e != nil {
		return false, fmt.Errorf("检查最新版本失败：%w", e)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("发布接口返回 HTTP %d", resp.StatusCode)
	}
	var release releaseInfo
	if e = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&release); e != nil {
		return false, fmt.Errorf("解析发布信息失败：%w", e)
	}
	if release.Draft || release.Prerelease || utils.ParseVersion(release.Version) == nil {
		return false, fmt.Errorf("发布接口返回了无效的正式版本")
	}
	u.release = &release
	return utils.CompareVersion(utils.ParseVersion(release.Version), utils.ParseVersion(ipgw.Version)), nil
}
func (u *UpdateHandler) download(rawURL string) (path string, err error) {
	parsed, e := url.Parse(rawURL)
	if e != nil || parsed.Scheme != "https" || parsed.Host != "github.com" || parsed.User != nil || !strings.HasPrefix(parsed.Path, "/"+ipgw.ReleaseRepo+"/releases/download/") {
		return "", fmt.Errorf("下载地址不在配置的发布仓库内")
	}
	resp, e := u.downloadClient.Get(rawURL)
	if e != nil {
		return "", e
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载返回 HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > 128<<20 {
		return "", fmt.Errorf("下载超出 128 MiB 限制")
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
		return path, fmt.Errorf("下载超出 128 MiB 限制")
	}
	if resp.ContentLength >= 0 && n != resp.ContentLength {
		return path, fmt.Errorf("下载不完整")
	}
	return path, nil
}
func verifyDigest(path, digest string) error {
	if !strings.HasPrefix(digest, "sha256:") {
		return fmt.Errorf("发布文件缺少 SHA-256 摘要")
	}
	expected, e := hex.DecodeString(strings.TrimPrefix(digest, "sha256:"))
	if e != nil || len(expected) != sha256.Size {
		return fmt.Errorf("发布文件的 SHA-256 摘要无效")
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
		return fmt.Errorf("下载文件的 SHA-256 与发布元数据不匹配")
	}
	return nil
}
func validateExecutable(path string) error {
	info, e := buildinfo.ReadFile(path)
	if e != nil {
		return fmt.Errorf("无效的 Go 可执行文件：%w", e)
	}
	if info.Main.Path != "github.com/ze-mu-zhou/SFR-ipgw" || (info.Path != "command-line-arguments" && info.Path != "github.com/ze-mu-zhou/SFR-ipgw/cmd/ipgw") {
		return fmt.Errorf("发布中包含意外的应用程序 %q", info.Path)
	}
	settings := map[string]string{}
	for _, s := range info.Settings {
		settings[s.Key] = s.Value
	}
	if settings["GOOS"] != runtime.GOOS || settings["GOARCH"] != runtime.GOARCH {
		return fmt.Errorf("发布可执行文件的平台与 %s/%s 不匹配", runtime.GOOS, runtime.GOARCH)
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
		return "", fmt.Errorf("备份可执行文件失败：%w", e)
	}
	if e = rename(candidate, current); e != nil {
		if rollbackErr := rename(backup, current); rollbackErr != nil {
			return backup, fmt.Errorf("安装失败：%v；回滚失败：%v；请从 %s 恢复可执行文件", e, rollbackErr, backup)
		}
		return "", fmt.Errorf("安装失败，已恢复原可执行文件：%w", e)
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
		return fmt.Errorf("发布中没有文件 %s", name)
	}
	if asset.Digest == "" {
		return fmt.Errorf("发布文件没有 SHA-256 摘要，请手动更新")
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
	console.InfoF("旧可执行文件已保留在 %s\n", backup)
	return nil
}
