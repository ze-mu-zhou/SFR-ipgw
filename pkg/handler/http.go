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
func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.TrimSpace(b.String())
}
func nodes(n *html.Node, tag string) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == tag {
			out = append(out, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return out
}
func pageDOM(body string) (*html.Node, error) { return html.Parse(strings.NewReader(body)) }
func pageFormatError() error {
	return errors.New("页面格式已变化或登录已过期：缺少必要字段")
}
func dashboardPage(client *http.Client, path string) (string, error) {
	resp, err := client.Get("https://ipgw.neu.edu.cn:8800" + path)
	if err != nil {
		return "", safeRequestError(err)
	}
	if resp.Request != nil && (resp.Request.URL.Host != "ipgw.neu.edu.cn:8800" || strings.Contains(resp.Request.URL.Path, "login")) {
		resp.Body.Close()
		return "", errors.New("计费系统会话已过期，请重新登录")
	}
	return responseBody(resp)
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
