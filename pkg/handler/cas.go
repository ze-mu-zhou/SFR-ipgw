package handler

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
)

const casLoginURL = "https://pass.neu.edu.cn/tpass/login"

var casKeyRE = regexp.MustCompile(`\b(?:const|let|var)\s+publicKeyStr\s*=\s*["']([A-Za-z0-9+/=]+)["']`)

func readCASResponse(resp *http.Response) (string, error) {
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("统一认证返回 HTTP %d", resp.StatusCode)
	}
	const limit = 1024 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return "", err
	}
	if len(body) > limit {
		return "", errors.New("统一认证页面过大，已停止登录")
	}
	return string(body), nil
}

// Use the public key from the school's current login script, rather than the
// plaintext concatenation used by neugo v0.3.1. Never retry a credential POST.
func loginCAS(client *http.Client, loginURL, username, password string) error {
	origin, err := url.Parse(loginURL)
	if err != nil || origin.Scheme != "https" || origin.Host == "" {
		return errors.New("统一认证地址不是有效的 HTTPS 地址")
	}
	guarded := *client
	previousRedirect := client.CheckRedirect
	credentialsSubmitted := false
	stoppedAfterAuthentication := false
	guarded.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" || req.URL.Host != origin.Host {
			// CAS may issue CASTGC and redirect to a portal after a successful
			// credential POST. We only need the CAS session, not that portal.
			// Stop before contacting it; never replay a POST across origins.
			if credentialsSubmitted && req.Method == http.MethodGet && req.Body == nil && hasCASTicket(guarded.Jar, origin) {
				stoppedAfterAuthentication = true
				return http.ErrUseLastResponse
			}
			return errors.New("认证重定向离开了可信站点，已停止登录")
		}
		if previousRedirect != nil {
			return previousRedirect(req, via)
		}
		if len(via) >= 10 {
			return errors.New("认证重定向次数过多")
		}
		return nil
	}
	client = &guarded
	resp, err := client.Get(loginURL)
	if err != nil {
		return safeRequestError(err)
	}
	base := resp.Request.URL
	body, err := readCASResponse(resp)
	if err != nil {
		return err
	}
	if base.Scheme != "https" {
		return errors.New("统一认证未使用 HTTPS，已停止登录")
	}
	doc, err := pageDOM(body)
	if err != nil {
		return err
	}
	fields := url.Values{}
	for _, tag := range nodes(doc, "input") {
		switch attr(tag, "name") {
		case "lt", "execution", "_eventId":
			fields.Set(attr(tag, "name"), attr(tag, "value"))
		}
	}
	if fields.Get("lt") == "" || fields.Get("execution") == "" {
		return errors.New("未找到统一认证登录表单，请在浏览器确认是否需要额外验证")
	}
	var scriptURL *url.URL
	for _, tag := range nodes(doc, "script") {
		src, parseErr := url.Parse(attr(tag, "src"))
		if parseErr != nil || !strings.HasSuffix(src.Path, "/login_neu.js") {
			continue
		}
		candidate := base.ResolveReference(src)
		if candidate.Scheme == base.Scheme && candidate.Host == base.Host {
			scriptURL = candidate
			break
		}
	}
	if scriptURL == nil {
		return errors.New("未找到学校登录脚本，登录页面可能已更新")
	}
	resp, err = client.Get(scriptURL.String())
	if err != nil {
		return safeRequestError(err)
	}
	if resp.Request.URL.Scheme != base.Scheme || resp.Request.URL.Host != base.Host {
		resp.Body.Close()
		return errors.New("登录脚本跳转到其他站点，已停止登录")
	}
	script, err := readCASResponse(resp)
	if err != nil {
		return err
	}
	match := casKeyRE.FindStringSubmatch(script)
	if len(match) != 2 {
		return errors.New("无法读取学校 RSA 公钥，登录脚本可能已更新")
	}
	der, err := base64.StdEncoding.DecodeString(match[1])
	if err != nil {
		return errors.New("学校 RSA 公钥编码无效")
	}
	parsed, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return errors.New("无法解析学校 RSA 公钥")
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return errors.New("学校登录公钥不是 RSA 公钥")
	}
	ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, key, []byte(username+password))
	if err != nil {
		return errors.New("无法加密登录信息，请检查账号密码长度")
	}
	fields.Set("rsa", base64.StdEncoding.EncodeToString(ciphertext))
	fields.Set("ul", strconv.Itoa(len(utf16.Encode([]rune(username)))))
	fields.Set("pl", strconv.Itoa(len(utf16.Encode([]rune(password)))))
	if fields.Get("_eventId") == "" {
		fields.Set("_eventId", "submit")
	}
	req, err := http.NewRequest(http.MethodPost, base.String(), strings.NewReader(fields.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", base.String())
	credentialsSubmitted = true
	resp, err = client.Do(req)
	if err != nil {
		return safeRequestError(err)
	}
	if stoppedAfterAuthentication {
		resp.Body.Close()
		return nil
	}
	body, err = readCASResponse(resp)
	if err != nil {
		return err
	}
	doc, err = pageDOM(body)
	if err != nil {
		return err
	}
	if titles := nodes(doc, "title"); len(titles) == 1 {
		switch nodeText(titles[0]) {
		case "系统提示":
			return errors.New("学校返回“系统提示”，不能据此判断账号被封；请在浏览器查看提示或完成额外验证")
		case "智慧东大--统一身份认证":
			return errors.New("登录未通过，请检查账号密码，或在浏览器确认是否需要验证码")
		}
	}
	if hasCASTicket(client.Jar, base) {
		return nil
	}
	return errors.New("未取得统一认证登录凭据，请在浏览器确认登录状态或额外验证要求")
}

func hasCASTicket(jar http.CookieJar, scope *url.URL) bool {
	if jar != nil {
		for _, cookie := range jar.Cookies(scope) {
			if cookie.Name == "CASTGC" && cookie.Value != "" {
				return true
			}
		}
	}
	return false
}
