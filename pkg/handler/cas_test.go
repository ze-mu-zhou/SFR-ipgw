package handler

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginCAS(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, result, wantError string
		missingKey, setCookie   bool
		redirectStatus          int
	}{
		{name: "encrypted login", result: "<title>登录成功</title>", setCookie: true},
		{name: "system message is not a ban", result: "<title>系统提示</title>", wantError: "不能据此判断账号被封"},
		{name: "unknown page is not success", result: "<title>维护中</title>", wantError: "未取得统一认证登录凭据"},
		{name: "missing key stops before submission", missingKey: true, wantError: "无法读取学校 RSA 公钥"},
		{name: "authenticated portal 302", setCookie: true, redirectStatus: http.StatusFound},
		{name: "authenticated portal 303", setCookie: true, redirectStatus: http.StatusSeeOther},
		{name: "redirect without ticket is not success", redirectStatus: http.StatusFound, wantError: "trusted origin"},
		{name: "never replay credentials after 307", setCookie: true, redirectStatus: http.StatusTemporaryRedirect, wantError: "trusted origin"},
		{name: "never replay credentials after 308", setCookie: true, redirectStatus: http.StatusPermanentRedirect, wantError: "trusted origin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			posts := 0
			outsideRequests := 0
			outside := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				outsideRequests++
				fmt.Fprint(w, "portal")
			}))
			defer outside.Close()
			const username, password = "20260001", "P&+空=word"
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/tpass/comm/neu/js/login_neu.js" {
					if !tc.missingKey {
						fmt.Fprintf(w, `const publicKeyStr = "%s";`, base64.StdEncoding.EncodeToString(der))
					}
					return
				}
				if r.Method == http.MethodGet {
					http.SetCookie(w, &http.Cookie{Name: "session", Value: "test-session", Path: "/"})
					fmt.Fprint(w, `<input value='LT-test&amp;token' name='lt'><input name="execution" value="e4s7"><input name="_eventId" value="submit"><script src='/tpass/comm/neu/js/login_neu.js?v=20260302'></script>`)
					return
				}
				posts++
				if _, err := r.Cookie("session"); err != nil {
					t.Error("login session cookie was not retained")
				}
				if err := r.ParseForm(); err != nil {
					t.Error(err)
					return
				}
				for name, want := range map[string]string{"lt": "LT-test&token", "execution": "e4s7", "_eventId": "submit", "ul": "8", "pl": "9"} {
					if r.PostForm.Get(name) != want {
						t.Errorf("form field %s differs", name)
					}
				}
				ciphertext, err := base64.StdEncoding.DecodeString(r.PostForm.Get("rsa"))
				if err != nil {
					t.Error("RSA field is not valid base64")
					return
				}
				plaintext, err := rsa.DecryptPKCS1v15(rand.Reader, key, ciphertext)
				if err != nil || string(plaintext) != username+password {
					t.Error("RSA payload does not match the current login protocol")
				}
				if tc.setCookie {
					http.SetCookie(w, &http.Cookie{Name: "CASTGC", Value: "test-ticket", Path: "/tpass/", Secure: true})
				}
				if tc.redirectStatus != 0 {
					http.Redirect(w, r, outside.URL+"/portal?ticket=synthetic-ticket", tc.redirectStatus)
					return
				}
				fmt.Fprint(w, tc.result)
			}))
			defer server.Close()
			client := server.Client()
			client.Jar, _ = cookiejar.New(nil)
			err := loginCAS(client, server.URL+"/tpass/login", username, password)
			if tc.wantError == "" && err != nil {
				t.Fatal(err)
			}
			if tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError)) {
				t.Fatalf("expected %q, got %v", tc.wantError, err)
			}
			wantPosts := 1
			if tc.missingKey {
				wantPosts = 0
			}
			if posts != wantPosts {
				t.Fatalf("expected %d credential submissions, got %d", wantPosts, posts)
			}
			if outsideRequests != 0 {
				t.Fatal("CAS login must not contact the external portal")
			}
		})
	}
}

func TestCASRejectsCrossOrigin307(t *testing.T) {
	redirected := false
	outside := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected = true }))
	defer outside.Close()
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, outside.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	client := origin.Client()
	client.Jar, _ = cookiejar.New(nil)
	if e := loginCAS(client, origin.URL, "dummy", "dummy"); e == nil {
		t.Fatal("cross-origin redirect accepted")
	}
	if redirected {
		t.Fatal("authentication request leaked to another origin")
	}
}
