package main

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sylvinus/agent-vm/internal/license"
)

type accountReply struct {
	Token   string              `json:"token"`
	Error   string              `json:"error"`
	Code    string              `json:"code"`
	Account license.AccountInfo `json:"account"`
}

func registrationRequest(handler http.Handler, address, username, device string) *httptest.ResponseRecorder {
	data, _ := json.Marshal(accountRequest{Username: username, Password: "fixture-password", Device: device})
	r := httptest.NewRequest(http.MethodPost, "/v1/auth/register", strings.NewReader(string(data)))
	r.RemoteAddr = address
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestRegistrationAllowsDifferentDevicesOnSharedNetwork(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(nil)
	mux := http.NewServeMux()
	registerAccountAPI(mux, license.NewStore(t.TempDir(), key))
	for i := 1; i <= 8; i++ {
		w := registrationRequest(mux, "203.0.113.5:4000", fmt.Sprintf("shared-user-%d", i), fmt.Sprintf("%064x", i))
		if w.Code != http.StatusOK {
			t.Fatalf("device %d on a shared network received HTTP %d", i, w.Code)
		}
	}
}

func TestRegistrationCooldownIsPerDeviceAndExpires(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(nil)
	now := time.Unix(100, 0)
	api := newAccountAPI(license.NewStore(t.TempDir(), key), func() time.Time { return now })
	handler := api.handle(false, api.register)
	device := strings.Repeat("a", 64)
	const address = "203.0.113.5:4000"
	if w := registrationRequest(handler, address, "original", device); w.Code != http.StatusOK {
		t.Fatalf("initial registration returned %d", w.Code)
	}
	for range 5 {
		if w := registrationRequest(handler, address, "original", device); w.Code != http.StatusConflict {
			t.Fatalf("duplicate registration returned %d", w.Code)
		}
	}
	now = now.Add(9*time.Minute + 45*time.Second)
	w := registrationRequest(handler, "203.0.113.6:4000", "new-name", device)
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "15" ||
		!strings.Contains(w.Body.String(), "15 秒") {
		t.Fatalf("missing device cooldown and exact retry time: %d; retry=%q", w.Code, w.Header().Get("Retry-After"))
	}
	if w := registrationRequest(handler, address, "other-device", strings.Repeat("b", 64)); w.Code != http.StatusOK {
		t.Fatalf("device cooldown blocked another device: %d", w.Code)
	}
	now = now.Add(15 * time.Second)
	if w := registrationRequest(handler, address, "new-name", device); w.Code != http.StatusOK {
		t.Fatalf("expired cooldown still blocked registration: %d", w.Code)
	}
}

func TestRegistrationSharedNetworkStillHasCeiling(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(nil)
	now := time.Unix(100, 0)
	api := newAccountAPI(license.NewStore(t.TempDir(), key), func() time.Time { return now })
	handler := api.handle(false, api.register)
	const address = "203.0.113.5:4000"
	for i := range registrationsPerNetwork {
		w := registrationRequest(handler, address, fmt.Sprintf("network-user-%d", i), fmt.Sprintf("%064x", i))
		if w.Code != http.StatusOK {
			t.Fatalf("accepted network allowance ended at %d: HTTP %d", i, w.Code)
		}
	}
	device := strings.Repeat("c", 64)
	if w := registrationRequest(handler, address, "network-overflow", device); w.Code != http.StatusTooManyRequests {
		t.Fatalf("shared network ceiling returned %d", w.Code)
	}
	if w := registrationRequest(handler, "203.0.113.7:4000", "other-network", device); w.Code != http.StatusOK {
		t.Fatalf("limited network affected a different network: %d", w.Code)
	}
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
	// Repeated registration on this device is still limited.
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
