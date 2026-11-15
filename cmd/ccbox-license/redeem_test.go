package main

import (
	"crypto/ed25519"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sylvinus/agent-vm/internal/license"
)

// Desktop clients read the status: 400 means the code cannot be used, 409 that
// another device owns it, anything else that the service is unavailable.
func TestRedeemStatuses(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(nil)
	store := license.NewStore(t.TempDir(), key)
	handler := redeemHandler(store)
	codes, err := store.IssueBatch(2)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Revoke(license.CodeID(codes[1]), time.Now()); err != nil {
		t.Fatal(err)
	}
	redeem := func(code, device string) *httptest.ResponseRecorder {
		body := `{"code":"` + code + `","device":"` + device + `"}`
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/redeem", strings.NewReader(body)))
		return w
	}
	a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)
	for _, tc := range []struct {
		code, device string
		status       int
		body         string
	}{
		{codes[0], a, 200, `"token"`},
		{codes[0], a, 200, `"token"`},
		{codes[0], b, 409, license.ErrUsed.Error()},
		{codes[1], a, 400, license.ErrRevoked.Error()},
		{"00000000-00000000-00000000-00000000", a, 400, license.ErrInvalid.Error()},
		{"not-a-code", a, 400, license.ErrInvalid.Error()},
		{codes[0], "short", 400, license.ErrInvalid.Error()},
	} {
		w := redeem(tc.code, tc.device)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.body) {
			t.Fatalf("redeem %s on %s… = %d %q", tc.code, tc.device[:4], w.Code, w.Body)
		}
	}
}
