package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionDoesNotFollowPlainHTTPRedirect(t *testing.T) {
	hits := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer source.Close()
	resp, err := newSession().Get(source.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound || hits != 0 {
		t.Fatalf("followed plaintext redirect: status=%d target hits=%d", resp.StatusCode, hits)
	}
}
