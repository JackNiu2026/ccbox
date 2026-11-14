package main

import (
	"crypto/ed25519"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sylvinus/agent-vm/internal/license"
)

func TestAdminIssue(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(nil)
	store := license.NewStore(t.TempDir(), key)
	handler := adminHandler(store, strings.Repeat("p", 32))
	request := func(method, origin, password string, form url.Values) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:8791/admin/license/", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if password != "" {
			r.SetBasicAuth("admin", password)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if got := request("GET", "", "", nil).Code; got != 401 {
		t.Fatalf("anonymous status = %d", got)
	}
	password := strings.Repeat("p", 32)
	origin := "http://127.0.0.1:8791"
	if got := request("POST", "https://other.example", password, nil).Code; got != 403 {
		t.Fatalf("cross-origin status = %d", got)
	}
	w := request("POST", origin, password, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "CCBox") {
		t.Fatalf("issue status = %d", w.Code)
	}
	code := regexp.MustCompile(`[A-F0-9]{8}(-[A-F0-9]{8}){3}`).FindString(w.Body.String())
	if _, err := store.Redeem(code, strings.Repeat("a", 64), time.Now()); err != nil {
		t.Fatal(err)
	}
	for _, count := range []string{"", "0", "-1", "1.5", "501", "abc"} {
		w := request("POST", origin, password, url.Values{"count": {count}})
		if w.Code != http.StatusBadRequest || strings.Contains(w.Body.String(), `id="issued"`) {
			t.Fatalf("invalid count %q returned %d", count, w.Code)
		}
	}
	w = request("POST", origin, password, url.Values{"count": {"3"}})
	codes := regexp.MustCompile(`[A-F0-9]{8}(-[A-F0-9]{8}){3}`).FindAllString(w.Body.String(), -1)
	if w.Code != http.StatusOK || len(codes) != 3 {
		t.Fatalf("expected three codes, got %d and status %d", len(codes), w.Code)
	}
	seen := map[string]bool{code: true}
	for _, code := range codes {
		if seen[code] {
			t.Fatal("duplicate code")
		}
		seen[code] = true
		if _, err := store.Redeem(code, strings.Repeat("b", 64), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
}
