package main

import (
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sylvinus/agent-vm/internal/license"
)

const testOrigin = "http://127.0.0.1:8791"

type adminClient struct {
	t      *testing.T
	mux    *http.ServeMux
	cookie *http.Cookie
}

func (c *adminClient) do(method, path, origin string, body any) *httptest.ResponseRecorder {
	c.t.Helper()
	var reader *strings.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = strings.NewReader(string(data))
	} else {
		reader = strings.NewReader("")
	}
	r := httptest.NewRequest(method, testOrigin+adminPrefix+path, reader)
	if method != http.MethodGet {
		r.Header.Set("Content-Type", "application/json")
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if c.cookie != nil {
		r.AddCookie(c.cookie)
	}
	w := httptest.NewRecorder()
	c.mux.ServeHTTP(w, r)
	return w
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
		t.Fatalf("invalid JSON %q: %v", w.Body.String(), err)
	}
	return value
}

func newTestAdmin(t *testing.T) (*adminClient, *license.Store, *adminServer) {
	_, key, _ := ed25519.GenerateKey(nil)
	dir := t.TempDir()
	store := license.NewStore(dir, key)
	server := newAdminServer(store, dir, strings.Repeat("p", 32), nil)
	mux := http.NewServeMux()
	server.register(mux)
	return &adminClient{t: t, mux: mux}, store, server
}

func TestAdminAuthentication(t *testing.T) {
	client, _, server := newTestAdmin(t)
	page := client.do("GET", "", "", nil)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "assets/admin.js") || !strings.Contains(page.Header().Get("Content-Security-Policy"), "script-src 'self'") {
		t.Fatalf("shell page: %d %q", page.Code, page.Header().Get("Content-Security-Policy"))
	}
	if got := client.do("GET", "assets/admin.js", "", nil); got.Code != 200 || !strings.HasPrefix(got.Header().Get("Content-Type"), "text/javascript") {
		t.Fatalf("script asset: %d", got.Code)
	}
	for _, name := range []string{"index.html", "..%2fadmin.go", "missing.js"} {
		if got := client.do("GET", "assets/"+name, "", nil); got.Code != 404 {
			t.Fatalf("asset %s: %d", name, got.Code)
		}
	}
	if got := client.do("GET", "api/overview", "", nil); got.Code != 401 {
		t.Fatalf("anonymous overview: %d", got.Code)
	}
	if got := decode[map[string]any](t, client.do("GET", "api/session", "", nil)); got["authenticated"] != false {
		t.Fatalf("anonymous session: %v", got)
	}
	if got := client.do("POST", "api/login", "https://evil.example", map[string]string{"password": strings.Repeat("p", 32)}); got.Code != 403 {
		t.Fatalf("cross-origin login: %d", got.Code)
	}
	if got := client.do("POST", "api/login", testOrigin, map[string]string{"password": "wrong"}); got.Code != 401 {
		t.Fatalf("wrong password: %d", got.Code)
	}
	login := client.do("POST", "api/login", testOrigin, map[string]string{"password": strings.Repeat("p", 32)})
	if login.Code != 200 {
		t.Fatalf("login: %d %s", login.Code, login.Body)
	}
	cookie := login.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != adminPrefix || cookie.Secure {
		t.Fatalf("session cookie attributes: %+v", cookie)
	}
	client.cookie = cookie
	session := decode[map[string]any](t, client.do("GET", "api/session", "", nil))
	if session["authenticated"] != true || session["max_batch"] != float64(license.MaxIssueBatch) {
		t.Fatalf("session: %v", session)
	}
	if got := client.do("POST", "api/codes", "", map[string]int{"count": 1}); got.Code != 403 {
		t.Fatalf("mutation without origin: %d", got.Code)
	}
	server.now = func() time.Time { return time.Now().Add(sessionIdle + time.Minute) }
	if got := client.do("GET", "api/overview", "", nil); got.Code != 401 {
		t.Fatalf("idle session accepted: %d", got.Code)
	}
	server.now = time.Now
	client.cookie = nil
	client.cookie = client.do("POST", "api/login", testOrigin, map[string]string{"password": strings.Repeat("p", 32)}).Result().Cookies()[0]
	if got := client.do("POST", "api/logout", testOrigin, map[string]any{}); got.Code != 200 {
		t.Fatalf("logout: %d", got.Code)
	}
	if got := client.do("GET", "api/overview", "", nil); got.Code != 401 {
		t.Fatalf("session survived logout: %d", got.Code)
	}
}

func TestAdminLoginRateLimit(t *testing.T) {
	client, _, _ := newTestAdmin(t)
	for range maxLoginFailures {
		client.do("POST", "api/login", testOrigin, map[string]string{"password": "wrong"})
	}
	if got := client.do("POST", "api/login", testOrigin, map[string]string{"password": strings.Repeat("p", 32)}); got.Code != http.StatusTooManyRequests {
		t.Fatalf("rate limit not enforced: %d", got.Code)
	}
}

func TestAdminIssueAndManage(t *testing.T) {
	client, store, _ := newTestAdmin(t)
	client.cookie = client.do("POST", "api/login", testOrigin, map[string]string{"password": strings.Repeat("p", 32)}).Result().Cookies()[0]
	for _, body := range []any{map[string]int{"count": 0}, map[string]int{"count": license.MaxIssueBatch + 1},
		map[string]any{"count": 1, "note": strings.Repeat("x", license.MaxNoteLength+1)}, map[string]any{"count": 1, "extra": true}} {
		if got := client.do("POST", "api/codes", testOrigin, body); got.Code != 400 {
			t.Fatalf("invalid issue %v: %d", body, got.Code)
		}
	}
	issued := decode[struct {
		Batch string   `json:"batch"`
		Codes []string `json:"codes"`
	}](t, client.do("POST", "api/codes", testOrigin, map[string]any{"count": 3, "note": "渠道 A"}))
	pattern := regexp.MustCompile(`^[A-F0-9]{8}(-[A-F0-9]{8}){3}$`)
	if len(issued.Codes) != 3 || issued.Batch == "" {
		t.Fatalf("issue result: %+v", issued)
	}
	for _, code := range issued.Codes {
		if !pattern.MatchString(code) {
			t.Fatalf("code format: %q", code)
		}
	}
	device := strings.Repeat("a", 64)
	if _, err := store.Redeem(issued.Codes[0], device, time.Now()); err != nil {
		t.Fatal(err)
	}
	lookup := client.do("POST", "api/codes/lookup", testOrigin, map[string]string{"code": strings.ToLower(issued.Codes[0])})
	found := decode[license.CodeInfo](t, lookup)
	if lookup.Code != 200 || found.Status != license.StatusRedeemed || found.Device != device || found.Batch != issued.Batch {
		t.Fatalf("lookup: %d %+v", lookup.Code, found)
	}
	if strings.Contains(lookup.Body.String(), "token") {
		t.Fatal("lookup exposes the signed grant")
	}
	if got := client.do("POST", "api/codes/lookup", testOrigin, map[string]string{"code": "00000000-00000000-00000000-00000000"}); got.Code != 404 {
		t.Fatalf("unknown lookup: %d", got.Code)
	}
	if got := client.do("POST", "api/codes/revoke", testOrigin, map[string]string{"id": found.ID}); got.Code != 400 {
		t.Fatalf("revoked a redeemed code: %d", got.Code)
	}
	unused := license.CodeID(issued.Codes[1])
	if got := decode[license.CodeInfo](t, client.do("POST", "api/codes/revoke", testOrigin, map[string]string{"id": unused})); got.Status != license.StatusRevoked {
		t.Fatalf("revoke: %+v", got)
	}
	if _, err := store.Redeem(issued.Codes[1], device, time.Now()); err == nil {
		t.Fatal("revoked code redeemed")
	}
	list := decode[struct {
		Items []license.CodeInfo `json:"items"`
		Total int                `json:"total"`
	}](t, client.do("GET", "api/codes?status=unused&batch="+issued.Batch, "", nil))
	if list.Total != 1 || list.Items[0].ID != license.CodeID(issued.Codes[2]) {
		t.Fatalf("filtered list: %+v", list)
	}
	hint := issued.Codes[2][len(issued.Codes[2])-4:]
	if got := decode[struct{ Total int }](t, client.do("GET", "api/codes?q="+hint, "", nil)); got.Total < 1 {
		t.Fatal("search by hint failed")
	}
	batches := decode[struct {
		Items []batchRow `json:"items"`
	}](t, client.do("GET", "api/batches", "", nil))
	if len(batches.Items) != 1 || batches.Items[0].Redeemed != 1 || batches.Items[0].Revoked != 1 || batches.Items[0].Unused != 1 || batches.Items[0].Note != "渠道 A" {
		t.Fatalf("batches: %+v", batches)
	}
	if got := decode[map[string]int](t, client.do("POST", "api/batches/revoke", testOrigin, map[string]string{"id": issued.Batch})); got["revoked"] != 1 {
		t.Fatalf("batch revoke: %v", got)
	}

	other := strings.Repeat("b", 64)
	if got := client.do("POST", "api/devices/transfer", testOrigin, map[string]string{"from": device, "to": device}); got.Code != 400 {
		t.Fatalf("self transfer: %d", got.Code)
	}
	transfer := decode[map[string]string](t, client.do("POST", "api/devices/transfer", testOrigin, map[string]string{"from": strings.ToUpper(device), "to": other}))
	if !pattern.MatchString(transfer["code"]) {
		t.Fatalf("transfer: %v", transfer)
	}
	if got := client.do("POST", "api/devices/transfer", testOrigin, map[string]string{"from": device, "to": other}); got.Code != 400 {
		t.Fatalf("repeated transfer: %d", got.Code)
	}
	renewal := decode[map[string]any](t, client.do("POST", "api/devices/renew", testOrigin, map[string]string{"device": other}))
	if renewal["code"] == nil || renewal["expires"].(float64) < float64(time.Now().Add(2*license.Duration-time.Hour).Unix()) {
		t.Fatalf("renewal: %v", renewal)
	}
	devices := decode[struct {
		Items []deviceRow `json:"items"`
	}](t, client.do("GET", "api/devices", "", nil))
	statuses := map[string]string{}
	for _, row := range devices.Items {
		statuses[row.Device] = row.Status
	}
	if statuses[device] != "transferred" || statuses[other] != "active" {
		t.Fatalf("devices: %+v", devices.Items)
	}
	overview := decode[struct {
		Codes   map[string]int `json:"codes"`
		Devices map[string]int `json:"devices"`
		Series  []dayCount     `json:"series"`
	}](t, client.do("GET", "api/overview", "", nil))
	if overview.Codes["issued"] != 3 || overview.Codes["redeemed"] != 1 || overview.Codes["revoked"] != 2 || overview.Codes["transfer"] != 1 ||
		overview.Devices["active"] != 1 || len(overview.Series) != 30 || overview.Series[29].Issued != 3 || overview.Series[29].Redeemed != 1 {
		t.Fatalf("overview: %+v", overview)
	}
	audit := decode[struct {
		Items []auditEntry `json:"items"`
	}](t, client.do("GET", "api/audit", "", nil))
	actions := []string{}
	for _, entry := range audit.Items {
		actions = append(actions, entry.Action)
	}
	want := "devices.renew devices.transfer batches.revoke codes.revoke codes.issue login"
	if strings.Join(actions, " ") != want {
		t.Fatalf("audit actions %v", actions)
	}
	for _, code := range issued.Codes {
		if strings.Contains(client.do("GET", "api/audit", "", nil).Body.String(), code) {
			t.Fatal("audit log contains an activation code")
		}
	}
}

func TestParseOrigins(t *testing.T) {
	origins, err := parseOrigins(" https://admin.example.com/ , http://127.0.0.1:9000")
	if err != nil || strings.Join(origins, " ") != "https://admin.example.com http://127.0.0.1:9000" {
		t.Fatalf("origins: %v %v", origins, err)
	}
	for _, bad := range []string{"ftp://x", "https://x/path", "x.example"} {
		if _, err := parseOrigins(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}
