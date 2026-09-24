package handler

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ze-mu-zhou/SFR-ipgw/pkg/model"
)

// Use the real client, cookie jar and redirect policy with an entirely local
// transport. Any unexpected request fails instead of reaching the campus network.
func campusFlowClient(t *testing.T, service testTransport) *http.Client {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	client := newSession()
	client.Transport = testTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme != "https" {
			t.Fatalf("request used cleartext transport: %s", r.URL)
		}
		if r.URL.Host == "pass.neu.edu.cn" {
			switch {
			case r.URL.Path == "/login_neu.js":
				return flowResponse(r, 200, fmt.Sprintf(`const publicKeyStr = "%s";`, base64.StdEncoding.EncodeToString(der))), nil
			case r.URL.Path == "/tpass/login" && r.Method == http.MethodPost:
				if err := r.ParseForm(); err != nil || r.PostForm.Get("rsa") == "" {
					t.Fatalf("missing encrypted credentials: %v", err)
				}
				resp := flowResponse(r, 200, "<title>登录成功</title>")
				resp.Header.Set("Set-Cookie", "CASTGC=test-ticket; Path=/tpass/; Secure")
				return resp, nil
			case r.URL.Path == "/tpass/login" && r.URL.Query().Get("service") == "":
				return flowResponse(r, 200, `<input name="lt" value="test-lt"><input name="execution" value="e1s1"><script src="/login_neu.js"></script>`), nil
			}
		}
		return service(r)
	})
	return client
}

func flowResponse(r *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}
}

func TestGatewayLoginFlow(t *testing.T) {
	for _, tc := range []struct {
		name, api, online string
		status            int
		bad               bool
	}{
		{"success", `{"code":0}`, `{"error":"ok","user_name":"alice","online_ip":"192.0.2.1"}`, 200, false},
		{"missing status", `{}`, "", 200, true},
		{"rejected", `{"code":1,"message":"denied"}`, "", 200, true},
		{"invalid JSON", `broken`, "", 200, true},
		{"HTTP failure", `unavailable`, "", 503, true},
		{"not online after success", `{"code":0}`, `{"error":"not_online_error","client_ip":"192.0.2.1"}`, 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ticketRequests := 0
			h := NewIpgwHandler()
			h.client = campusFlowClient(t, func(r *http.Request) (*http.Response, error) {
				switch r.URL.Host + r.URL.Path {
				case "ipgw.neu.edu.cn/":
					resp := flowResponse(r, 302, "")
					resp.Header.Set("Location", "https://ipgw.neu.edu.cn/srun_portal_pc?ac_id=1")
					return resp, nil
				case "ipgw.neu.edu.cn/srun_portal_pc":
					return flowResponse(r, 200, "portal"), nil
				case "pass.neu.edu.cn/tpass/login":
					if r.URL.Query().Get("service") != "http://ipgw.neu.edu.cn/srun_portal_sso?ac_id=1" {
						t.Fatalf("wrong CAS service: %s", r.URL)
					}
					if _, err := r.Cookie("CASTGC"); err != nil {
						t.Fatal("CAS session was not preserved")
					}
					resp := flowResponse(r, 302, "")
					resp.Header.Set("Location", "http://ipgw.neu.edu.cn/srun_portal_sso?ac_id=1&ticket=ST-test")
					return resp, nil
				case "ipgw.neu.edu.cn/srun_portal_sso":
					ticketRequests++
					if r.URL.Query().Get("ticket") != "ST-test" {
						t.Fatal("ticket was lost during HTTPS rewrite")
					}
					return flowResponse(r, 200, "portal"), nil
				case "ipgw.neu.edu.cn/v1/srun_portal_sso":
					return flowResponse(r, tc.status, tc.api), nil
				case "ipgw.neu.edu.cn/cgi-bin/rad_user_info":
					return flowResponse(r, 200, tc.online), nil
				default:
					t.Fatalf("unexpected request: %s", r.URL)
					return nil, nil
				}
			})
			err := h.Login(&model.Account{Username: "alice", Password: "test-password"})
			if (err != nil) != tc.bad || ticketRequests != 1 {
				t.Fatalf("error=%v ticketRequests=%d", err, ticketRequests)
			}
			if !tc.bad && (h.GetInfo().Username != "alice" || h.GetInfo().IP != "192.0.2.1") {
				t.Fatalf("online identity not confirmed: %+v", h.GetInfo())
			}
		})
	}
}

func TestGatewayLogoutFlow(t *testing.T) {
	for _, body := range []string{"logout_ok", "rejected"} {
		h := NewIpgwHandler()
		h.info.Username = "alice+test"
		h.client.Transport = testTransport(func(r *http.Request) (*http.Response, error) {
			if r.URL.Query().Get("username") != "alice+test" || r.URL.Query().Get("action") != "logout" || r.Header.Get("Referer") == "" {
				t.Fatalf("invalid logout request: %s", r.URL)
			}
			return flowResponse(r, 200, body), nil
		})
		if err := h.Logout(); (err == nil) != (body == "logout_ok") {
			t.Fatalf("body=%s error=%v", body, err)
		}
	}
}

func TestDashboardLoginFlow(t *testing.T) {
	homeRequests := 0
	d := NewDashboardHandler()
	d.cachedDashboardIndexContent = "stale previous session"
	d.client = campusFlowClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "ipgw.neu.edu.cn:8800" {
			t.Fatalf("unexpected host: %s", r.URL)
		}
		switch r.URL.Path {
		case "/sso/neusoft/index":
			return flowResponse(r, 200, "dashboard"), nil
		case "/home":
			homeRequests++
			return flowResponse(r, 200, `<div><label>用户名</label>alice</div><div><label>姓名</label>Test User</div>`+packageTable("10")), nil
		default:
			t.Fatalf("unexpected request: %s", r.URL)
			return nil, nil
		}
	})
	if err := d.Login(&model.Account{Username: "alice", Password: "test-password"}); err != nil {
		t.Fatal(err)
	}
	basic, err := d.GetBasic()
	if err != nil || basic.ID != "alice" {
		t.Fatalf("basic=%+v error=%v", basic, err)
	}
	if _, err := d.GetPackage(); err != nil {
		t.Fatal(err)
	}
	if homeRequests != 1 {
		t.Fatalf("dashboard cache not reused: %d requests", homeRequests)
	}
}
