package handler

import (
	"bytes"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
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

const maxDownloadSize = 128 << 20

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
	executablePath func() (path, dir string, err error)
}

type downloader struct {
	io.Reader
	total, current int64
	lastPrint      time.Time
}

// 进度最多每 200ms 打印一次，避免输出重定向到文件时写出成千上万行
const progressInterval = 200 * time.Millisecond

func (d *downloader) Read(p []byte) (int, error) {
	n, err := d.Reader.Read(p)
	d.current += int64(n)
	now := time.Now()
	if err == nil && now.Sub(d.lastPrint) < progressInterval {
		return n, err
	}
	d.lastPrint = now
	if d.total > 0 {
		console.Infof("\r已下载 %.2f%%", float64(d.current)/float64(d.total)*100)
	} else {
		console.Infof("\r已下载 %d 字节", d.current)
	}
	return n, err
}

func httpsOnlyRedirect(req *http.Request, via []*http.Request) error {
	if req.URL.Scheme != "https" {
		return errors.New("更新重定向到非 HTTPS 地址，已拒绝")
	}
	if len(via) >= 10 {
		return errors.New("更新重定向次数过多")
	}
	return nil
}

// NewUpdateHandler 创建更新处理器。API 检查使用总超时；下载只限制连接和响应头，
// 响应体读取不设总时长，慢速网络下也能完成大文件下载。
func NewUpdateHandler() *UpdateHandler {
	return &UpdateHandler{
		executablePath: utils.ExecutablePathAndDir,
		apiClient:      &http.Client{Timeout: 30 * time.Second, CheckRedirect: httpsOnlyRedirect},
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
		return errors.New("本地/自定义构建已禁用自动更新；请手动重新构建，或使用配置了兼容发布仓库的正式版本")
	}
	return nil
}

func (u *UpdateHandler) CheckLatestVersion() (bool, error) {
	if err := updateAllowed(); err != nil {
		return false, err
	}
	u.release = nil
	resp, err := u.apiClient.Get("https://api.github.com/repos/" + ipgw.ReleaseRepo + "/releases/latest")
	if err != nil {
		return false, fmt.Errorf("检查最新版本失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("发布接口返回 HTTP %d", resp.StatusCode)
	}
	var release releaseInfo
	if err = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&release); err != nil {
		return false, fmt.Errorf("解析发布信息失败：%w", err)
	}
	if release.Draft || release.Prerelease || utils.ParseVersion(release.Version) == nil {
		return false, errors.New("发布接口返回了无效的正式版本")
	}
	u.release = &release
	return utils.CompareVersion(utils.ParseVersion(release.Version), utils.ParseVersion(ipgw.Version)), nil
}

func (u *UpdateHandler) download(rawURL string) (path string, err error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" || parsed.User != nil || !strings.HasPrefix(parsed.Path, "/"+ipgw.ReleaseRepo+"/releases/download/") {
		return "", errors.New("下载地址不在配置的发布仓库内")
	}
	resp, err := u.downloadClient.Get(rawURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载返回 HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxDownloadSize {
		return "", errors.New("下载超出 128 MiB 限制")
	}
	file, err := os.CreateTemp("", "ipgw.release.*")
	if err != nil {
		return "", err
	}
	path = file.Name()
	defer func() {
		if err != nil {
			os.Remove(path)
		}
	}()
	written, copyErr := io.Copy(file, &downloader{Reader: io.LimitReader(resp.Body, maxDownloadSize+1), total: resp.ContentLength})
	console.Infoln()
	closeErr := file.Close()
	if copyErr != nil {
		return path, copyErr
	}
	if closeErr != nil {
		return path, closeErr
	}
	if written > maxDownloadSize {
		return path, errors.New("下载超出 128 MiB 限制")
	}
	if resp.ContentLength >= 0 && written != resp.ContentLength {
		return path, errors.New("下载不完整")
	}
	return path, nil
}

func verifyDigest(path, digest string) error {
	if !strings.HasPrefix(digest, "sha256:") {
		return errors.New("发布文件缺少 SHA-256 摘要")
	}
	expected, err := hex.DecodeString(strings.TrimPrefix(digest, "sha256:"))
	if err != nil || len(expected) != sha256.Size {
		return errors.New("发布文件的 SHA-256 摘要无效")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err = io.Copy(hasher, file); err != nil {
		return err
	}
	if !bytes.Equal(hasher.Sum(nil), expected) {
		return errors.New("下载文件的 SHA-256 与发布元数据不匹配")
	}
	return nil
}

func validateExecutable(path string) error {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return fmt.Errorf("无效的 Go 可执行文件：%w", err)
	}
	if info.Main.Path != "github.com/ze-mu-zhou/SFR-ipgw" || (info.Path != "command-line-arguments" && info.Path != "github.com/ze-mu-zhou/SFR-ipgw/cmd/ipgw") {
		return fmt.Errorf("发布中包含意外的应用程序 %q", info.Path)
	}
	settings := map[string]string{}
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	if settings["GOOS"] != runtime.GOOS || settings["GOARCH"] != runtime.GOARCH {
		return fmt.Errorf("发布可执行文件的平台与 %s/%s 不匹配", runtime.GOOS, runtime.GOARCH)
	}
	return nil
}

// replaceExecutable keeps the old executable as a recovery copy. On Windows it
// can remain locked until this process exits, so it is not deleted automatically.
func replaceExecutable(current, candidate string, rename func(string, string) error) (string, error) {
	temp, err := os.CreateTemp(filepath.Dir(current), ".ipgw-backup-*")
	if err != nil {
		return "", err
	}
	backup := temp.Name()
	if err = temp.Close(); err != nil {
		return "", err
	}
	if err = os.Remove(backup); err != nil {
		return "", err
	}
	if err = rename(current, backup); err != nil {
		return "", fmt.Errorf("备份可执行文件失败：%w", err)
	}
	if err = rename(candidate, current); err != nil {
		if rollbackErr := rename(backup, current); rollbackErr != nil {
			return backup, fmt.Errorf("安装失败：%v；回滚失败：%v；请从 %s 恢复可执行文件", err, rollbackErr, backup)
		}
		return "", fmt.Errorf("安装失败，已恢复原可执行文件：%w", err)
	}
	return backup, nil
}

func (u *UpdateHandler) Update() error {
	if err := updateAllowed(); err != nil {
		return err
	}
	if u.release == nil {
		newer, err := u.CheckLatestVersion()
		if err != nil {
			return err
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
		return errors.New("发布文件没有 SHA-256 摘要，请手动更新")
	}
	downloaded, err := u.download(asset.URL)
	if err != nil {
		return err
	}
	defer os.Remove(downloaded)
	if err = verifyDigest(downloaded, asset.Digest); err != nil {
		return err
	}
	current, dir, err := u.executablePath()
	if err != nil {
		return err
	}
	stage, err := os.MkdirTemp(dir, ".ipgw-update-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err = utils.Unzip(downloaded, stage); err != nil {
		return err
	}
	executable := "ipgw"
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	candidate := filepath.Join(stage, executable)
	if err = validateExecutable(candidate); err != nil {
		return err
	}
	mode := os.FileMode(0755)
	if info, statErr := os.Stat(current); statErr == nil {
		mode = info.Mode().Perm()
	}
	if err = os.Chmod(candidate, mode); err != nil {
		return err
	}
	backup, err := replaceExecutable(current, candidate, os.Rename)
	if err != nil {
		return err
	}
	console.Infof("旧可执行文件已保留在 %s\n", backup)
	return nil
}
