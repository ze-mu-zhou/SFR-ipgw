package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/model"
	"net/http"
	"net/url"
	"strings"
)

type gatewayInfo struct {
	Error    *string  `json:"error"`
	Username string   `json:"user_name"`
	OnlineIP string   `json:"online_ip"`
	ClientIP string   `json:"client_ip"`
	Bytes    *int64   `json:"sum_bytes"`
	Seconds  *int64   `json:"sum_seconds"`
	Balance  *float64 `json:"user_balance"`
}
type IpgwHandler struct {
	info      *model.Info
	client    *http.Client
	oriInfo   gatewayInfo
	kickReady bool
}

func NewIpgwHandler() *IpgwHandler          { return &IpgwHandler{info: &model.Info{}, client: newSession()} }
func (h *IpgwHandler) GetInfo() *model.Info { return h.info }
func (h *IpgwHandler) Login(a *model.Account) error {
	p, err := a.GetPassword()
	if err != nil {
		return err
	}
	b, e := h.login(a.Username, p)
	if e != nil {
		return e
	}
	var r struct {
		Code    *int   `json:"code"`
		Message string `json:"message"`
	}
	if e = json.Unmarshal([]byte(b), &r); e != nil {
		return errors.New("网关登录响应无效")
	}
	if r.Code == nil {
		return errors.New("网关登录响应缺少状态码")
	}
	if *r.Code != 0 {
		if r.Message != "" {
			return fmt.Errorf("网关登录失败：%s（代码 %d）", r.Message, *r.Code)
		}
		return fmt.Errorf("网关登录失败（代码 %d）", *r.Code)
	}
	if err := h.ParseBasicInfo(); err != nil {
		return err
	}
	if h.info.Username == "" {
		return errors.New("登录后网关未确认在线账号")
	}
	return nil
}
func (h *IpgwHandler) NEUAuth(u, p string) error {
	h.kickReady = false
	return loginCAS(h.client, casLoginURL, u, p)
}
func (h *IpgwHandler) login(u, p string) (string, error) {
	if e := h.NEUAuth(u, p); e != nil {
		return "", e
	}
	return h.requestLoginApi()
}
func (h *IpgwHandler) FetchUsageInfo() error {
	if e := h.getJsonIpgwData(); e != nil {
		return e
	}
	d := h.oriInfo
	if *d.Error != "ok" {
		return errors.New("网关账号未登录")
	}
	if d.Bytes == nil || d.Seconds == nil || d.Balance == nil || *d.Bytes < 0 || *d.Seconds < 0 {
		return errors.New("网关用量响应缺少字段或字段无效")
	}
	h.info.Traffic = *d.Bytes
	h.info.UsedTime = *d.Seconds
	h.info.Balance = *d.Balance
	return nil
}
func (h *IpgwHandler) requestLoginApi() (string, error) {
	r, e := h.client.Get("https://ipgw.neu.edu.cn/")
	if e != nil {
		return "", safeRequestError(e)
	}
	q := r.Request.URL.RawQuery
	if _, e = responseBody(r); e != nil {
		return "", e
	}
	service := "http://ipgw.neu.edu.cn/srun_portal_sso?" + q
	r, e = h.client.Get(casLoginURL + "?" + url.Values{"service": {service}}.Encode())
	if e != nil {
		return "", safeRequestError(e)
	}
	if r.StatusCode >= 300 && r.StatusCode < 400 {
		// 票据绑定 http service，网关按此校验；仅将传输改写为 https
		loc, le := r.Location()
		r.Body.Close()
		if le != nil {
			return "", errors.New("统一认证未返回网关票据")
		}
		loc.Scheme = "https"
		if r, e = h.client.Get(loc.String()); e != nil {
			return "", safeRequestError(e)
		}
	}
	u := r.Request.URL
	if _, e = responseBody(r); e != nil {
		return "", e
	}
	if u.Hostname() != "ipgw.neu.edu.cn" || !strings.HasPrefix(u.Path, "/srun_portal") {
		return "", errors.New("统一认证未返回网关票据")
	}
	r, e = h.client.Get("https://ipgw.neu.edu.cn/v1" + u.RequestURI())
	if e != nil {
		return "", safeRequestError(e)
	}
	return responseBody(r)
}
func (h *IpgwHandler) getJsonIpgwData() error {
	req, e := http.NewRequest("GET", "https://ipgw.neu.edu.cn/cgi-bin/rad_user_info", nil)
	if e != nil {
		return e
	}
	req.Header.Set("Accept", "application/json")
	r, e := h.client.Do(req)
	if e != nil {
		return safeRequestError(e)
	}
	b, e := responseBody(r)
	if e != nil {
		return e
	}
	var d gatewayInfo
	if e = json.Unmarshal([]byte(b), &d); e != nil {
		return errors.New("网关信息响应无效")
	}
	if d.Error == nil {
		return errors.New("网关响应缺少状态")
	}
	if *d.Error == "ok" {
		if !required(d.Username, d.OnlineIP) {
			return errors.New("网关响应缺少账号或 IP")
		}
	} else if *d.Error != "not_online_error" || d.ClientIP == "" {
		return errors.New("网关返回了失败状态")
	}
	h.oriInfo = d
	return nil
}
func (h *IpgwHandler) ParseBasicInfo() error {
	if e := h.getJsonIpgwData(); e != nil {
		return e
	}
	h.info.Username = ""
	h.info.IP = h.oriInfo.ClientIP
	if *h.oriInfo.Error == "ok" {
		h.info.Username = h.oriInfo.Username
		h.info.IP = h.oriInfo.OnlineIP
	}
	return nil
}
func (h *IpgwHandler) Logout() error {
	req, e := http.NewRequest("GET", "https://ipgw.neu.edu.cn/cgi-bin/srun_portal?"+url.Values{"action": {"logout"}, "username": {h.info.Username}}.Encode(), nil)
	if e != nil {
		return e
	}
	req.Header.Set("Referer", "https://ipgw.neu.edu.cn/srun_portal_success?ac_id=1")
	r, e := h.client.Do(req)
	if e != nil {
		return safeRequestError(e)
	}
	b, e := responseBody(r)
	if e != nil {
		return e
	}
	if strings.TrimSpace(b) != "logout_ok" {
		return errors.New("网关拒绝了注销请求")
	}
	return nil
}
func (h *IpgwHandler) CheckConnection() (connected, loggedIn bool, err error) {
	if err = h.ParseBasicInfo(); err != nil {
		return false, false, err
	}
	return h.info.IP != "", h.info.Username != "", nil
}
func (h *IpgwHandler) Kick(sid string) (bool, error) {
	if sid == "" {
		return false, errors.New("需要设备会话 ID")
	}
	if !h.kickReady {
		if _, e := dashboardPage(h.client, "/sso/neusoft/index"); e != nil {
			return false, e
		}
		h.kickReady = true
	}
	b, e := dashboardPage(h.client, "/home")
	if e != nil {
		h.kickReady = false
		return false, e
	}
	doc, e := pageDOM(b)
	if e != nil {
		return false, e
	}
	token := ""
	for _, n := range nodes(doc, "meta") {
		if attr(n, "name") == "csrf-token" {
			token = attr(n, "content")
		}
	}
	if token == "" {
		h.kickReady = false
		return false, pageFormatError()
	}
	req, e := http.NewRequest("POST", "https://ipgw.neu.edu.cn:8800/home/delete?"+url.Values{"id": {sid}}.Encode(), strings.NewReader(url.Values{"_csrf-8800": {token}}.Encode()))
	if e != nil {
		return false, e
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "https://ipgw.neu.edu.cn:8800/home/index")
	r, e := h.client.Do(req)
	if e != nil {
		return false, safeRequestError(e)
	}
	b, e = responseBody(r)
	if e != nil {
		return false, e
	}
	if !strings.Contains(b, "下线请求已发出") {
		return false, errors.New("服务器未确认设备已下线")
	}
	return true, nil
}
