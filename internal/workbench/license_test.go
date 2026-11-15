package workbench

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sylvinus/agent-vm/internal/guestos"
	"github.com/sylvinus/agent-vm/internal/license"
)

type activationTransport func(*http.Request) (*http.Response, error)

func (f activationTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDesktopLicenseActivationAndAdmission(t *testing.T) {
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	key, client := licensePublicKey, activationHTTP
	t.Cleanup(func() { licensePublicKey, activationHTTP = key, client })
	licensePublicKey = base64.RawURLEncoding.EncodeToString(public)
	s := &Server{profile: t.TempDir(), token: "test-desktop-token"}
	store := license.NewStore(t.TempDir(), private)
	code, err := store.Issue()
	if err != nil {
		t.Fatal(err)
	}
	device, err := license.DeviceID()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	activationHTTP = &http.Client{Transport: activationTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != licenseEndpoint || r.Method != http.MethodPost {
			t.Fatalf("unexpected activation request: %v", r)
		}
		var input struct{ Code, Device string }
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Code != code || input.Device != device {
			t.Fatalf("unexpected activation input: %v", err)
		}
		token, err := store.Redeem(input.Code, input.Device, now)
		if err != nil {
			t.Fatal(err)
		}
		result := httptest.NewRecorder()
		jsonReply(result, map[string]string{"token": token})
		return result.Result(), nil
	})}
	if _, err := s.submitJob("start", func() error { return nil }); err != errLicenseRequired {
		t.Fatalf("inactive start should be blocked: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/desktop-license", strings.NewReader(`{"code":"`+code+`"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.desktopLicense(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("non-desktop caller: %d", w.Code)
	}
	req.AddCookie(&http.Cookie{Name: s.cookieName() + "-desktop", Value: trustedViewProof(s.token)})
	w = httptest.NewRecorder()
	s.desktopLicense(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("activation failed: %s", w.Body)
	}
	// A fresh controller can read the same grant without contacting the service.
	activationHTTP = &http.Client{Transport: activationTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("offline admission attempted a network request")
		return nil, nil
	})}
	restarted := &Server{profile: s.profile}
	status, err := restarted.licenseStatus()
	if err != nil || status.State != "active" || status.Expires != now.Add(license.Duration).Unix() {
		t.Fatalf("offline status: %+v %v", status, err)
	}
	if err := restarted.requireLicense(); err != nil {
		t.Fatalf("offline grant should be accepted: %v", err)
	}
	native := &Server{origin: "http://127.0.0.1:8989", token: "native-token", session: "native-session", windowsGuest: &guestos.WindowsProfile{}}
	nativeReq := httptest.NewRequest(http.MethodGet, native.origin+"/api/desktop-license", nil)
	nativeReq.AddCookie(&http.Cookie{Name: native.cookieName(), Value: native.session})
	nativeReply := httptest.NewRecorder()
	native.handler().ServeHTTP(nativeReply, nativeReq)
	if nativeReply.Code == http.StatusConflict {
		t.Fatalf("native guest rejected the authorization route: %s", nativeReply.Body)
	}
	expiredCode, err := store.Issue()
	if err != nil {
		t.Fatal(err)
	}
	expired, err := store.Redeem(expiredCode, strings.Repeat("b", 64), now.Add(-2*license.Duration))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifyLicense(expired, device); err == nil {
		t.Fatal("a signed grant from a different device was accepted")
	}
	expiredStore := license.NewStore(t.TempDir(), private)
	expiredCode, err = expiredStore.Issue()
	if err != nil {
		t.Fatal(err)
	}
	expired, err = expiredStore.Redeem(expiredCode, device, now.Add(-2*license.Duration))
	if err != nil {
		t.Fatal(err)
	}
	if err := writePrivate(filepath.Join(s.profile, "license.json"), expired); err != nil {
		t.Fatal(err)
	}
	blocked := httptest.NewRecorder()
	restarted.launch(blocked, "start", func() error { t.Error("expired license started an environment"); return nil })
	if blocked.Code != http.StatusPaymentRequired {
		t.Fatalf("expired start returned %d", blocked.Code)
	}
}

func TestRedeemLicenseReportsRevokedCodes(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   error
	}{
		{http.StatusBadRequest, license.ErrRevoked.Error() + "\n", license.ErrRevoked},
		{http.StatusBadRequest, license.ErrInvalid.Error() + "\n", license.ErrInvalid},
		{http.StatusConflict, license.ErrUsed.Error() + "\n", license.ErrUsed},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, strings.TrimSpace(tc.body), tc.status)
		}))
		previous := licenseEndpoint
		licenseEndpoint = server.URL
		_, err := redeemLicense(context.Background(), "CODE", strings.Repeat("a", 64))
		licenseEndpoint = previous
		server.Close()
		if !errors.Is(err, tc.want) {
			t.Fatalf("status %d %q: got %v, want %v", tc.status, tc.body, err, tc.want)
		}
	}
}
