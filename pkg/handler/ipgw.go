package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/ze-mu-zhou/SFR-ipgw/pkg/model"
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

type IPGWHandler struct {
	info      *model.Info
	client    *http.Client
	gateway   gatewayInfo
	kickReady bool
}

func NewIPGWHandler() *IPGWHandler {
	return &IPGWHandler{info: &model.Info{}, client: newSession()}
}

func (h *IPGWHandler) Info() *model.Info { return h.info }

func (h *IPGWHandler) Login(account *model.Account) error {
	password, err := account.GetPassword()
	if err != nil {
		return err
	}
	body, err := h.login(account.Username, password)
	if err != nil {
		return err
	}
	var result struct {
		Code    *int   `json:"code"`
		Message string `json:"message"`
	}
	if err = json.Unmarshal([]byte(body), &result); err != nil {
		return errors.New("网关登录响应无效")
	}
	if result.Code == nil {
		return errors.New("网关登录响应缺少状态码")
	}
	if *result.Code != 0 {
		if result.Message != "" {
			return fmt.Errorf("网关登录失败：%s（代码 %d）", result.Message, *result.Code)
		}
		return fmt.Errorf("网关登录失败（代码 %d）", *result.Code)
	}
	if err := h.ParseBasicInfo(); err != nil {
		return err
	}
	if h.info.Username == "" {
		return errors.New("登录后网关未确认在线账号")
	}
	return nil
}

func (h *IPGWHandler) NEUAuth(username, password string) error {
	h.kickReady = false
	return loginCAS(h.client, casLoginURL, username, password)
}

func (h *IPGWHandler) login(username, password string) (string, error) {
	if err := h.NEUAuth(username, password); err != nil {
		return "", err
	}
	return h.requestLoginAPI()
}

func (h *IPGWHandler) FetchUsageInfo() error {
	if err := h.fetchGatewayInfo(); err != nil {
		return err
	}
	gateway := h.gateway
	if *gateway.Error != "ok" {
		return errors.New("网关账号未登录")
	}
	if gateway.Bytes == nil || gateway.Seconds == nil || gateway.Balance == nil || *gateway.Bytes < 0 || *gateway.Seconds < 0 {
		return errors.New("网关用量响应缺少字段或字段无效")
	}
	h.info.Traffic = *gateway.Bytes
	h.info.UsedTime = *gateway.Seconds
	h.info.Balance = *gateway.Balance
	return nil
}

func (h *IPGWHandler) requestLoginAPI() (string, error) {
	resp, err := h.client.Get(gatewayURL + "/")
	if err != nil {
		return "", safeRequestError(err)
	}
	query := resp.Request.URL.RawQuery
	if _, err = responseBody(resp); err != nil {
		return "", err
	}
	service := "http://" + gatewayHost + "/srun_portal_sso?" + query
	resp, err = h.client.Get(casLoginURL + "?" + url.Values{"service": {service}}.Encode())
	if err != nil {
		return "", safeRequestError(err)
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		// 票据绑定 http service，网关按此校验；仅将传输改写为 https
		location, locationErr := resp.Location()
		resp.Body.Close()
		if locationErr != nil {
			return "", errors.New("统一认证未返回网关票据")
		}
		location.Scheme = "https"
		if resp, err = h.client.Get(location.String()); err != nil {
			return "", safeRequestError(err)
		}
	}
	portal := resp.Request.URL
	if _, err = responseBody(resp); err != nil {
		return "", err
	}
	if portal.Hostname() != gatewayHost || !strings.HasPrefix(portal.Path, "/srun_portal") {
		return "", errors.New("统一认证未返回网关票据")
	}
	resp, err = h.client.Get(gatewayURL + "/v1" + portal.RequestURI())
	if err != nil {
		return "", safeRequestError(err)
	}
	return responseBody(resp)
}

func (h *IPGWHandler) fetchGatewayInfo() error {
	req, err := http.NewRequest(http.MethodGet, gatewayURL+"/cgi-bin/rad_user_info", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := h.client.Do(req)
	if err != nil {
		return safeRequestError(err)
	}
	body, err := responseBody(resp)
	if err != nil {
		return err
	}
	var gateway gatewayInfo
	if err = json.Unmarshal([]byte(body), &gateway); err != nil {
		return errors.New("网关信息响应无效")
	}
	if gateway.Error == nil {
		return errors.New("网关响应缺少状态")
	}
	if *gateway.Error == "ok" {
		if !required(gateway.Username, gateway.OnlineIP) {
			return errors.New("网关响应缺少账号或 IP")
		}
	} else if *gateway.Error != "not_online_error" || gateway.ClientIP == "" {
		return errors.New("网关返回了失败状态")
	}
	h.gateway = gateway
	return nil
}

func (h *IPGWHandler) ParseBasicInfo() error {
	if err := h.fetchGatewayInfo(); err != nil {
		return err
	}
	h.info.Username = ""
	h.info.IP = h.gateway.ClientIP
	if *h.gateway.Error == "ok" {
		h.info.Username = h.gateway.Username
		h.info.IP = h.gateway.OnlineIP
	}
	return nil
}

func (h *IPGWHandler) Logout() error {
	req, err := http.NewRequest(http.MethodGet, gatewayURL+"/cgi-bin/srun_portal?"+url.Values{"action": {"logout"}, "username": {h.info.Username}}.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Referer", gatewayURL+"/srun_portal_success?ac_id=1")
	resp, err := h.client.Do(req)
	if err != nil {
		return safeRequestError(err)
	}
	body, err := responseBody(resp)
	if err != nil {
		return err
	}
	if strings.TrimSpace(body) != "logout_ok" {
		return errors.New("网关拒绝了注销请求")
	}
	return nil
}

func (h *IPGWHandler) CheckConnection() (connected, loggedIn bool, err error) {
	if err = h.ParseBasicInfo(); err != nil {
		return false, false, err
	}
	return h.info.IP != "", h.info.Username != "", nil
}

func (h *IPGWHandler) Kick(sid string) (bool, error) {
	if sid == "" {
		return false, errors.New("需要设备会话 ID")
	}
	if !h.kickReady {
		if _, err := dashboardPage(h.client, "/sso/neusoft/index"); err != nil {
			return false, err
		}
		h.kickReady = true
	}
	body, err := dashboardPage(h.client, "/home")
	if err != nil {
		h.kickReady = false
		return false, err
	}
	doc, err := pageDOM(body)
	if err != nil {
		return false, err
	}
	token := ""
	for _, meta := range nodes(doc, "meta") {
		if attr(meta, "name") == "csrf-token" {
			token = attr(meta, "content")
		}
	}
	if token == "" {
		h.kickReady = false
		return false, pageFormatError()
	}
	req, err := http.NewRequest(http.MethodPost, dashboardURL+"/home/delete?"+url.Values{"id": {sid}}.Encode(), strings.NewReader(url.Values{"_csrf-8800": {token}}.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", dashboardURL+"/home/index")
	resp, err := h.client.Do(req)
	if err != nil {
		return false, safeRequestError(err)
	}
	body, err = responseBody(resp)
	if err != nil {
		return false, err
	}
	if !strings.Contains(body, "下线请求已发出") {
		return false, errors.New("服务器未确认设备已下线")
	}
	return true, nil
}
