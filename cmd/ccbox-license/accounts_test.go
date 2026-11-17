package main

import (
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sylvinus/agent-vm/internal/license"
)

type accountReply struct {
	Token   string              `json:"token"`
	Error   string              `json:"error"`
	Code    string              `json:"code"`
	Account license.AccountInfo `json:"account"`
}

func TestAccountEndpoints(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(nil)
	store := license.NewStore(t.TempDir(), key)
	mux := http.NewServeMux()
	registerAccountAPI(mux, store)
	device := strings.Repeat("a", 64)
	call := func(path, token string, body map[string]string) (int, accountReply) {
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(data)))
		r.RemoteAddr = "203.0.113.5:4000"
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		var reply accountReply
		_ = json.Unmarshal(w.Body.Bytes(), &reply)
		return w.Code, reply
	}
	credentials := map[string]string{"username": "grace", "password": "password1", "device": device, "device_name": "Laptop", "platform": "macos"}
	status, registered := call("/v1/auth/register", "", credentials)
	if status != 200 || len(registered.Token) != 64 || registered.Account.Username != "grace" {
		t.Fatalf("register: %d %+v", status, registered)
	}
	if status, reply := call("/v1/auth/register", "", credentials); status != http.StatusConflict || reply.Code != "username_taken" {
		t.Fatalf("duplicate register: %d %+v", status, reply)
	}
	if status, reply := call("/v1/auth/register", "", map[string]string{"username": "x", "password": "password1", "device": device}); status != 400 || reply.Code != "invalid_username" {
		t.Fatalf("invalid name: %d %+v", status, reply)
	}
	if status, reply := call("/v1/auth/login", "", map[string]string{"username": "grace", "password": "wrong-pass", "device": device}); status != 401 || reply.Code != "credentials" {
		t.Fatalf("wrong password: %d %+v", status, reply)
	}
	token := registered.Token
	session := map[string]string{"device": device}
	if status, reply := call("/v1/account/grant", token, session); status != http.StatusForbidden || reply.Code != "no_entitlement" {
		t.Fatalf("grant without term: %d %+v", status, reply)
	}
	code, _ := store.Issue()
	if status, reply := call("/v1/account/redeem", token, map[string]string{"device": device, "code": code}); status != 200 || !reply.Account.Active {
		t.Fatalf("redeem: %d %+v", status, reply)
	}
	if status, reply := call("/v1/account/redeem", token, map[string]string{"device": device, "code": code}); status != http.StatusConflict || reply.Code != "code_used" {
		t.Fatalf("redeem twice: %d %+v", status, reply)
	}
	status, granted := call("/v1/account/grant", token, session)
	if status != 200 {
		t.Fatalf("grant: %d %+v", status, granted)
	}
	if grant, err := license.Verify(pub, granted.Token, device); err != nil || grant.Account != granted.Account.ID {
		t.Fatalf("grant verify: %+v %v", grant, err)
	}
	if status, reply := call("/v1/account", token, map[string]string{"device": strings.Repeat("b", 64)}); status != 401 || reply.Code != "session" {
		t.Fatalf("token from another device: %d %+v", status, reply)
	}
	if status, reply := call("/v1/account", "", session); status != 401 {
		t.Fatalf("missing token: %d %+v", status, reply)
	}
	if status, _ := call("/v1/account", token, map[string]string{"device": device, "extra": "x"}); status != 400 {
		t.Fatalf("unknown field accepted: %d", status)
	}
	if status, _ := call("/v1/auth/logout", token, session); status != 200 {
		t.Fatalf("logout: %d", status)
	}
	if status, _ := call("/v1/account", token, session); status != 401 {
		t.Fatalf("session survived logout: %d", status)
	}
	// Registration is limited per address.
	limited := false
	for i := range 6 {
		body := map[string]string{"username": "user" + string(rune('a'+i)) + "xx", "password": "password1", "device": device}
		if status, reply := call("/v1/auth/register", "", body); status == http.StatusTooManyRequests && reply.Code == "rate_limited" {
			limited = true
		}
	}
	if !limited {
		t.Fatal("registration not rate limited")
	}
}

func TestMaxDevicesFromEnv(t *testing.T) {
	if n, err := maxDevicesFromEnv(""); n != 0 || err != nil {
		t.Fatal(n, err)
	}
	if n, err := maxDevicesFromEnv(" 3 "); n != 3 || err != nil {
		t.Fatal(n, err)
	}
	for _, bad := range []string{"0", "21", "two"} {
		if _, err := maxDevicesFromEnv(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}
