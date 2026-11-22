package workbench

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sylvinus/agent-vm/internal/guestos"
	"github.com/sylvinus/agent-vm/internal/license"
)

// fakeAccountService answers the desktop's account requests from a real
// license store, mirroring the service's status codes.
func fakeAccountService(t *testing.T, store *license.Store) *http.Client {
	return &http.Client{Transport: activationTransport(func(r *http.Request) (*http.Response, error) {
		var input map[string]string
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatalf("request body: %v", err)
		}
		token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		now := time.Now()
		var reply any
		var err error
		switch strings.TrimPrefix(r.URL.String(), strings.TrimSuffix(licenseEndpoint, "/v1/redeem")) {
		case "/v1/auth/register":
			var info license.AccountInfo
			token, info, err = store.Register(input["username"], input["password"], input["device"], license.DeviceMeta{Name: input["device_name"]}, now)
			reply = map[string]any{"token": token, "account": info}
		case "/v1/auth/login":
			var info license.AccountInfo
			token, info, err = store.Login(input["username"], input["password"], input["device"], license.DeviceMeta{}, now)
			reply = map[string]any{"token": token, "account": info}
		case "/v1/auth/logout":
			err = store.Logout(token)
			reply = map[string]bool{"ok": true}
		case "/v1/auth/reset":
			err = store.ResetPassword(input["username"], input["code"], input["password"], now)
			reply = map[string]bool{"ok": true}
		case "/v1/account":
			var info license.AccountInfo
			info, err = store.Account(token, input["device"], now)
			reply = map[string]any{"account": info}
		case "/v1/account/grant":
			var signed string
			signed, _, err = store.AccountGrant(token, input["device"], now)
			reply = map[string]string{"token": signed}
		case "/v1/account/redeem":
			_, err = store.RedeemToAccount(token, input["device"], input["code"], now)
			reply = map[string]bool{"ok": true}
		case "/v1/account/password":
			err = store.ChangePassword(token, input["device"], input["old_password"], input["password"], now)
			reply = map[string]bool{"ok": true}
		default:
			t.Fatalf("unexpected request %s", r.URL)
		}
		w := httptest.NewRecorder()
		if err != nil {
			status, code := http.StatusBadRequest, "invalid_request"
			for _, known := range []struct {
				err    error
				status int
				code   string
			}{
				{license.ErrCredentials, 401, "credentials"}, {license.ErrSession, 401, "session"},
				{license.ErrNoEntitlement, 403, "no_entitlement"}, {license.ErrUsernameTaken, 409, "username_taken"},
			} {
				if errors.Is(err, known.err) {
					status, code = known.status, known.code
				}
			}
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error(), "code": code})
			return w.Result(), nil
		}
		jsonReply(w, reply)
		return w.Result(), nil
	})}
}

func onlineTestServer(t *testing.T) *Server {
	t.Helper()
	s := &Server{profile: t.TempDir(), token: "test-desktop-token"}
	account, err := loadLocalAccount(s.profile)
	if err != nil {
		t.Fatal(err)
	}
	s.account = account
	return s
}

func TestOnlineAccountRegisterRedeemAndSignIn(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(nil)
	key, client := licensePublicKey, activationHTTP
	t.Cleanup(func() { licensePublicKey, activationHTTP = key, client })
	licensePublicKey = base64.RawURLEncoding.EncodeToString(public)
	store := license.NewStore(t.TempDir(), private)
	activationHTTP = fakeAccountService(t, store)
	request := httptest.NewRequest(http.MethodPost, "/api/account/create", nil)

	s := onlineTestServer(t)
	if !s.onlineEnabled() {
		t.Fatal("online accounts should be enabled in a licensed build")
	}
	if err := s.createWithOnline("register", "x", "password1", "", request); err == nil {
		t.Fatal("invalid local name accepted")
	}
	if err := s.createWithOnline("register", "Alice-1", "password1", "", request); err != nil {
		t.Fatalf("register: %v", err)
	}
	record, token, err := s.onlineSession()
	if err != nil || record.Username != "Alice-1" || len(token) != 64 {
		t.Fatalf("session: %+v %v", record, err)
	}
	if raw, _ := os.ReadFile(s.onlinePath()); strings.Contains(string(raw), token) {
		t.Fatal("session token stored in plain text")
	}
	if err := s.requireLicense(); !errors.Is(err, errLicenseRequired) {
		t.Fatalf("account without term admitted: %v", err)
	}
	code, _ := store.Issue()
	view, err := s.redeemOnline(context.Background(), code)
	if err != nil || view.State != "active" || view.Expires > time.Now().Add(license.AccountGrantTerm).Unix() {
		t.Fatalf("redeem: %+v %v", view, err)
	}
	if err := s.requireLicense(); err != nil {
		t.Fatalf("redeemed account blocked: %v", err)
	}
	if err := s.createWithOnline("register", "other", "password1", "", request); err == nil {
		t.Fatal("second local account created")
	}

	// The password changes online and locally together.
	if err := s.changeOnlinePassword(context.Background(), "wrong-pass", "password2"); err == nil {
		t.Fatal("wrong current password accepted")
	}
	if err := s.changeOnlinePassword(context.Background(), "password1", "password2"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Login("alice-1", "password2", strings.Repeat("e", 64), license.DeviceMeta{}, time.Now()); err != nil {
		t.Fatalf("online password not changed: %v", err)
	}

	// A new device signs in with the same account and gets its own local
	// account under the canonical name.
	other := onlineTestServer(t)
	if err := other.createWithOnline("login", "ALICE-1", "wrong-pass", "", request); onlineCode(err) != "credentials" {
		t.Fatalf("wrong password sign-in: %v", err)
	}
	if other.account.exists() {
		t.Fatal("failed sign-in created a local account")
	}
	if err := other.createWithOnline("login", "ALICE-1", "password2", "", request); err != nil {
		t.Fatalf("sign in: %v", err)
	}
	wantLocal := "Alice-1"
	if other.macAccountHost() {
		wantLocal = "alice_1"
	}
	if other.account.view().Username != wantLocal {
		t.Fatalf("local name: %q", other.account.view().Username)
	}

	// Locked: the sealed token is unreadable, and the grant renewal waits.
	other.account.lock()
	if _, _, err := other.onlineSession(); err == nil {
		t.Fatal("session readable while locked")
	}
}

func TestOnlineAccountsOffWithoutLicenseKey(t *testing.T) {
	key := licensePublicKey
	t.Cleanup(func() { licensePublicKey = key })
	licensePublicKey = ""
	s := onlineTestServer(t)
	if s.onlineEnabled() {
		t.Fatal("online accounts enabled without a license key")
	}
	request := httptest.NewRequest(http.MethodPost, "/api/account/create", nil)
	if err := s.createWithOnline("login", "alice", "password1", "", request); !errors.Is(err, errOnlineDisabled) {
		t.Fatalf("login without service: %v", err)
	}
	if err := s.createWithOnline("register", "alice", "password1", "", request); err != nil {
		t.Fatalf("local-only create: %v", err)
	}
	if _, err := os.Stat(s.onlinePath()); !os.IsNotExist(err) {
		t.Fatal("local-only create wrote an online session")
	}
}

func TestOnlinePasswordResetFromLockedDevice(t *testing.T) {
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	key, client := licensePublicKey, activationHTTP
	t.Cleanup(func() { licensePublicKey, activationHTTP = key, client })
	licensePublicKey = base64.RawURLEncoding.EncodeToString(public)
	store := license.NewStore(t.TempDir(), private)
	activationHTTP = fakeAccountService(t, store)

	s := testServer(t)
	s.account, err = loadLocalAccount(s.profile)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.account.lock)
	const registration = `{"username":"test_user","password":"original-test-password","confirm_password":"original-test-password"}`
	created := request(s, http.MethodPost, "/api/account/create", s.origin, registration, true)
	if created.Code != http.StatusOK || !s.account.view().Unlocked || strings.Contains(created.Body.String(), "recovery") {
		t.Fatal("registration failed or exposed retired recovery state", created.Code)
	}
	record, _, err := s.onlineSession()
	if err != nil || record == nil {
		t.Fatal("missing online account session", err)
	}
	code, _, err := store.IssueReset(record.User, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	s.account.lock()
	body, err := json.Marshal(map[string]string{"username": "test_user", "reset_code": code, "password": "reset-online-password", "confirm_password": "reset-online-password"})
	if err != nil {
		t.Fatal(err)
	}
	if w := request(s, http.MethodPost, "/api/account/online-reset", s.origin, string(body), true); w.Code != http.StatusOK || s.account.view().Unlocked {
		t.Fatal("online reset failed or unlocked the device", w.Code)
	}
	if _, _, err := store.Login("test_user", "reset-online-password", strings.Repeat("e", 64), license.DeviceMeta{}, time.Now()); err != nil {
		t.Fatal("new password cannot sign in online", err)
	}
	if err := s.account.unlock("original-test-password"); err != nil {
		t.Fatal("online reset changed the device password", err)
	}
	for _, action := range []string{"recovery-generate", "recovery-reset"} {
		if w := request(s, http.MethodPost, "/api/account/"+action, s.origin, `{}`, true); w.Code != http.StatusNotFound {
			t.Fatalf("retired endpoint %s returned %d", action, w.Code)
		}
	}
}

func TestMacLocalNameIsAValidMacOSLogin(t *testing.T) {
	for online, want := range map[string]string{
		"alice":                                "alice",
		"Alice-1":                              "alice_1",
		"j.doe":                                "j_doe",
		"7seas":                                "u7seas",
		"_x":                                   "u_x",
		"Root":                                 "cage_root",
		"GUEST":                                "cage_guest",
		"a.b-c_d9":                             "a_b_c_d9",
		"abcdefghijklmnopqrstuvwxyz0123456789": "abcdefghijklmnopqrstuvwxyz01234",
	} {
		got := macLocalName(online)
		if got != want || !guestos.ValidMacOSAccountName(got) {
			t.Errorf("macLocalName(%q) = %q, want %q", online, got, want)
		}
	}
}
