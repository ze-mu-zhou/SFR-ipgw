package handler

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// 学校服务器地址。网关（gateway）、计费系统（dashboard）与统一认证（CAS）
// 的域名或端口变化时只需修改这里。
const (
	gatewayHost   = "ipgw.neu.edu.cn"
	gatewayURL    = "https://" + gatewayHost
	dashboardHost = gatewayHost + ":8800"
	dashboardURL  = "https://" + dashboardHost
	casLoginURL   = "https://pass.neu.edu.cn/tpass/login"
)

func newSession() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, Timeout: 30 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		// CAS 票据绑定的 service 是 http 地址；拦截明文跳转，由调用方改写为 https
		if req.URL.Scheme != "https" {
			return http.ErrUseLastResponse
		}
		if len(via) >= 10 {
			return errors.New("重定向次数过多")
		}
		return nil
	}}
}

func responseBody(resp *http.Response) (string, error) {
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("服务器返回 HTTP %d", resp.StatusCode)
	}
	const limit = 4 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return "", err
	}
	if len(data) > limit {
		return "", errors.New("响应过大，已停止读取")
	}
	return string(data), nil
}

func attr(node *html.Node, key string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == key {
			return attribute.Val
		}
	}
	return ""
}

func nodeText(root *html.Node) string {
	var text strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			text.WriteString(node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return strings.TrimSpace(text.String())
}

func nodes(root *html.Node, tag string) []*html.Node {
	var matches []*html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == tag {
			matches = append(matches, node)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return matches
}

func pageDOM(body string) (*html.Node, error) { return html.Parse(strings.NewReader(body)) }

func pageFormatError() error {
	return errors.New("页面格式已变化或登录已过期：缺少必要字段")
}

func dashboardPage(client *http.Client, path string) (string, error) {
	resp, err := client.Get(dashboardURL + path)
	if err != nil {
		return "", safeRequestError(err)
	}
	if resp.Request != nil && isDashboardLogin(resp.Request.URL) {
		resp.Body.Close()
		return "", errors.New("计费系统会话已过期，请重新登录")
	}
	// 跳到 http 登录页时 CheckRedirect 会停在 3xx 上，Request.URL 仍是原地址，只能看 Location
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		resp.Body.Close()
		if location, err := resp.Location(); err == nil && isDashboardLogin(location) {
			return "", errors.New("计费系统会话已过期，请重新登录")
		}
		return "", fmt.Errorf("服务器返回 HTTP %d", resp.StatusCode)
	}
	return responseBody(resp)
}

func isDashboardLogin(u *url.URL) bool {
	return u.Host != dashboardHost || strings.Contains(u.Path, "login")
}

// Network errors may carry CAS tickets in their URL. Preserve the cause only.
func safeRequestError(err error) error {
	for {
		var requestError *url.Error
		if !errors.As(err, &requestError) || requestError.Err == nil {
			return err
		}
		err = requestError.Err
	}
}
