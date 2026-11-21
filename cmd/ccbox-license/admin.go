package main

import (
	"cmp"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sylvinus/agent-vm/internal/license"
)

//go:embed web
var adminFiles embed.FS

const (
	adminPrefix      = "/admin/license/"
	adminCookie      = "ccbox_admin"
	sessionIdle      = 45 * time.Minute
	maxSessions      = 32
	loginWindow      = 15 * time.Minute
	maxLoginFailures = 8
	defaultPageSize  = 50
	maxPageSize      = 200
	adminCSP         = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; font-src 'self'; form-action 'none'; frame-ancestors 'none'; base-uri 'none'"
)

var defaultAdminOrigins = []string{"http://127.0.0.1:8791", "http://localhost:8791", "https://ccbox.org"}

type session struct {
	seen   time.Time
	user   string
	secure bool
}

type adminServer struct {
	store    *license.Store
	username [32]byte
	password [32]byte
	user     string
	origins  []string
	audit    *auditLog
	now      func() time.Time

	mu       sync.Mutex
	sessions map[string]*session
	failures map[string][]time.Time
}

// validUsername accepts 1–64 printable characters without spaces.
func validUsername(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for _, r := range name {
		if r <= ' ' || r == 0x7f {
			return false
		}
	}
	return true
}

func newAdminServer(store *license.Store, stateDir, username, password string, origins []string) *adminServer {
	if len(origins) == 0 {
		origins = defaultAdminOrigins
	}
	return &adminServer{
		store: store, username: sha256.Sum256([]byte(username)), password: sha256.Sum256([]byte(password)), user: username, origins: origins,
		audit: &auditLog{dir: stateDir}, now: time.Now,
		sessions: map[string]*session{}, failures: map[string][]time.Time{},
	}
}

// parseOrigins reads a comma-separated list of exact browser origins.
func parseOrigins(value string) ([]string, error) {
	var origins []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimRight(strings.TrimSpace(item), "/")
		if item == "" {
			continue
		}
		parsed, err := url.Parse(item)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" {
			return nil, errors.New("invalid administrator origin: " + item)
		}
		origins = append(origins, parsed.Scheme+"://"+parsed.Host)
	}
	return origins, nil
}

func (a *adminServer) register(mux *http.ServeMux) {
	mux.HandleFunc("GET "+adminPrefix+"{$}", a.page)
	mux.HandleFunc("GET "+adminPrefix+"assets/{name}", a.asset)
	mux.HandleFunc("GET "+adminPrefix+"api/session", a.api(false, a.sessionInfo))
	mux.HandleFunc("POST "+adminPrefix+"api/login", a.login)
	mux.HandleFunc("POST "+adminPrefix+"api/logout", a.api(true, a.logout))
	mux.HandleFunc("GET "+adminPrefix+"api/overview", a.api(true, a.overview))
	mux.HandleFunc("GET "+adminPrefix+"api/codes", a.api(true, a.listCodes))
	mux.HandleFunc("POST "+adminPrefix+"api/codes", a.api(true, a.issueCodes))
	mux.HandleFunc("POST "+adminPrefix+"api/codes/lookup", a.api(true, a.lookupCode))
	mux.HandleFunc("POST "+adminPrefix+"api/codes/revoke", a.api(true, a.revokeCode))
	mux.HandleFunc("GET "+adminPrefix+"api/batches", a.api(true, a.listBatches))
	mux.HandleFunc("POST "+adminPrefix+"api/batches/revoke", a.api(true, a.revokeBatch))
	mux.HandleFunc("GET "+adminPrefix+"api/devices", a.api(true, a.listDevices))
	mux.HandleFunc("POST "+adminPrefix+"api/devices/transfer", a.api(true, a.transfer))
	mux.HandleFunc("POST "+adminPrefix+"api/devices/renew", a.api(true, a.renew))
	mux.HandleFunc("GET "+adminPrefix+"api/audit", a.api(true, a.listAudit))
	a.registerUsers(mux)
}

func securityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", adminCSP)
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
}

func (a *adminServer) page(w http.ResponseWriter, r *http.Request) {
	securityHeaders(w)
	data, err := adminFiles.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "missing administrator page", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (a *adminServer) asset(w http.ResponseWriter, r *http.Request) {
	securityHeaders(w)
	name := r.PathValue("name")
	if name == "index.html" || !fs.ValidPath(name) || strings.Contains(name, "/") {
		http.NotFound(w, r)
		return
	}
	data, err := adminFiles.ReadFile("web/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	types := map[string]string{".css": "text/css; charset=utf-8", ".js": "text/javascript; charset=utf-8", ".svg": "image/svg+xml",
		".woff2": "font/woff2", ".txt": "text/plain; charset=utf-8"}
	contentType, ok := types[path.Ext(name)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

type apiError struct {
	status  int
	message string
}

func (e apiError) Error() string { return e.message }

func badRequest(message string) error { return apiError{http.StatusBadRequest, message} }

type apiHandler func(*http.Request, string) (any, error)

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	var known apiError
	switch {
	case errors.As(err, &known):
		writeJSON(w, known.status, map[string]string{"error": known.message})
	case errors.Is(err, license.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	default:
		log.Printf("license administration failed: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "授权账本暂时不可用，请稍后重试"})
	}
}

// api wraps JSON endpoints: it applies security headers, checks the session
// and, for state-changing requests, the browser origin and content type.
func (a *adminServer) api(authenticated bool, handler apiHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		securityHeaders(w)
		if r.Method != http.MethodGet {
			if err := a.checkOrigin(r); err != nil {
				writeError(w, err)
				return
			}
		}
		token := a.session(w, r)
		if authenticated && token == "" {
			writeError(w, apiError{http.StatusUnauthorized, "登录已过期，请重新登录"})
			return
		}
		result, err := handler(r, token)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func (a *adminServer) checkOrigin(r *http.Request) error {
	origin := r.Header.Get("Origin")
	parsed, err := url.Parse(origin)
	if origin == "" || err != nil || parsed.Host != r.Host || !slices.Contains(a.origins, origin) {
		return apiError{http.StatusForbidden, "请求来源无效"}
	}
	if media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); media != "application/json" {
		return apiError{http.StatusUnsupportedMediaType, "请求格式无效"}
	}
	return nil
}

func decodeBody(r *http.Request, value any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 8<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil || decoder.Decode(new(any)) != io.EOF {
		return badRequest("请求内容无效")
	}
	return nil
}

func clientAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	// Only a reverse proxy on a trusted network can supply the visitor address.
	if ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			parts := strings.Split(forwarded, ",")
			if candidate := net.ParseIP(strings.TrimSpace(parts[len(parts)-1])); candidate != nil {
				return candidate.String()
			}
		}
	}
	return host
}

func (a *adminServer) session(w http.ResponseWriter, r *http.Request) string {
	cookie, err := r.Cookie(adminCookie)
	if err != nil || len(cookie.Value) != 64 {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	current := a.sessions[cookie.Value]
	if current == nil {
		return ""
	}
	if now.Sub(current.seen) >= sessionIdle {
		delete(a.sessions, cookie.Value)
		return ""
	}
	current.seen = now
	setAdminSessionCookie(w, cookie.Value, current.secure)
	return cookie.Value
}

func setAdminSessionCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: adminCookie, Value: token, Path: adminPrefix, HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteStrictMode, MaxAge: int(sessionIdle / time.Second)})
}

func (a *adminServer) sessionInfo(r *http.Request, token string) (any, error) {
	if token == "" {
		return map[string]any{"authenticated": false}, nil
	}
	a.mu.Lock()
	current := a.sessions[token]
	if current == nil {
		a.mu.Unlock()
		return map[string]any{"authenticated": false}, nil
	}
	expires, user := current.seen.Add(sessionIdle).Unix(), current.user
	a.mu.Unlock()
	return map[string]any{"authenticated": true, "expires": expires, "user": user, "idle": int(sessionIdle / time.Second),
		"duration_days": int(license.Duration / (24 * time.Hour)), "max_batch": license.MaxIssueBatch, "max_note": license.MaxNoteLength}, nil
}

func (a *adminServer) login(w http.ResponseWriter, r *http.Request) {
	securityHeaders(w)
	if err := a.checkOrigin(r); err != nil {
		writeError(w, err)
		return
	}
	address := clientAddress(r)
	now := a.now()
	a.mu.Lock()
	recent := slices.DeleteFunc(a.failures[address], func(at time.Time) bool { return now.Sub(at) > loginWindow })
	a.failures[address] = recent
	blocked := len(recent) >= maxLoginFailures
	a.mu.Unlock()
	if blocked {
		w.Header().Set("Retry-After", strconv.Itoa(int(loginWindow/time.Second)))
		writeError(w, apiError{http.StatusTooManyRequests, "登录失败次数过多，请 15 分钟后再试"})
		return
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeBody(r, &input); err != nil {
		writeError(w, err)
		return
	}
	// Compare both digests every time so timing reveals neither field.
	userDigest := sha256.Sum256([]byte(input.Username))
	passwordDigest := sha256.Sum256([]byte(input.Password))
	userOK := subtle.ConstantTimeCompare(userDigest[:], a.username[:])
	passwordOK := subtle.ConstantTimeCompare(passwordDigest[:], a.password[:])
	if userOK&passwordOK != 1 {
		a.mu.Lock()
		a.failures[address] = append(a.failures[address], now)
		a.mu.Unlock()
		a.audit.record(now, "login.failed", "", address)
		time.Sleep(400 * time.Millisecond)
		writeError(w, apiError{http.StatusUnauthorized, "账号或密码不正确"})
		return
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		writeError(w, err)
		return
	}
	token := hex.EncodeToString(raw[:])
	secure := strings.HasPrefix(r.Header.Get("Origin"), "https://")
	a.mu.Lock()
	delete(a.failures, address)
	for key, value := range a.sessions {
		if now.Sub(value.seen) >= sessionIdle {
			delete(a.sessions, key)
		}
	}
	for len(a.sessions) >= maxSessions {
		var oldest string
		for key, value := range a.sessions {
			if oldest == "" || value.seen.Before(a.sessions[oldest].seen) {
				oldest = key
			}
		}
		delete(a.sessions, oldest)
	}
	a.sessions[token] = &session{seen: now, user: a.user, secure: secure}
	a.mu.Unlock()
	setAdminSessionCookie(w, token, secure)
	a.audit.record(now, "login", a.user, address)
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true})
}

func (a *adminServer) logout(r *http.Request, token string) (any, error) {
	a.mu.Lock()
	user := ""
	if current := a.sessions[token]; current != nil {
		user = current.user
	}
	delete(a.sessions, token)
	a.mu.Unlock()
	a.audit.record(a.now(), "logout", user, clientAddress(r))
	return map[string]bool{"ok": true}, nil
}

// --- Overview ---------------------------------------------------------------

type dayCount struct {
	Day      string `json:"day"`
	Issued   int    `json:"issued"`
	Redeemed int    `json:"redeemed"`
}

func (a *adminServer) overview(r *http.Request, _ string) (any, error) {
	snapshot, err := a.store.Snapshot()
	if err != nil {
		return nil, err
	}
	now := a.now()
	totals := map[string]int{}
	for _, code := range snapshot.Codes {
		if code.Kind == license.KindCode {
			totals["issued"]++
			totals[code.Status]++
		} else {
			totals[code.Kind]++
		}
	}
	const days = 30
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(days - 1))
	series := make([]dayCount, days)
	for i := range series {
		series[i].Day = start.AddDate(0, 0, i).Format("2006-01-02")
	}
	bucket := func(at int64) int {
		if at == 0 {
			return -1
		}
		moment := time.Unix(at, 0).In(now.Location())
		if moment.Before(start) {
			return -1
		}
		day := time.Date(moment.Year(), moment.Month(), moment.Day(), 0, 0, 0, 0, now.Location())
		index := int(day.Sub(start).Hours()/24 + 0.5)
		if index < 0 || index >= days {
			return -1
		}
		return index
	}
	for _, code := range snapshot.Codes {
		if code.Kind != license.KindCode {
			continue
		}
		if i := bucket(code.Issued); i >= 0 {
			series[i].Issued++
		}
		if i := bucket(code.Redeemed); i >= 0 {
			series[i].Redeemed++
		}
	}
	devices := deviceRows(snapshot, now)
	deviceTotals := map[string]int{}
	var expiring []deviceRow
	for _, device := range devices {
		deviceTotals[device.Status]++
		if device.Status == "active" && device.Expires-now.Unix() <= 7*24*3600 {
			expiring = append(expiring, device)
		}
	}
	slices.SortFunc(expiring, func(x, y deviceRow) int { return cmp.Compare(x.Expires, y.Expires) })
	expiringTotal := len(expiring)
	if len(expiring) > 8 {
		expiring = expiring[:8]
	}
	batches := batchRows(snapshot)
	if len(batches) > 5 {
		batches = batches[:5]
	}
	users, err := a.store.Users(now)
	if err != nil {
		return nil, err
	}
	return map[string]any{"codes": totals, "devices": deviceTotals, "series": series, "users": userTotals(users, now),
		"expiring": expiring, "expiring_total": expiringTotal, "batches": batches, "generated": now.Unix()}, nil
}

// --- Codes ------------------------------------------------------------------

func pageParams(r *http.Request) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > maxPageSize {
		size = defaultPageSize
	}
	return page, size
}

func paginate[T any](items []T, page, size int) map[string]any {
	total := len(items)
	from := min((page-1)*size, total)
	to := min(from+size, total)
	return map[string]any{"items": items[from:to], "total": total, "page": page, "size": size}
}

func (a *adminServer) listCodes(r *http.Request, _ string) (any, error) {
	snapshot, err := a.store.Snapshot()
	if err != nil {
		return nil, err
	}
	query := r.URL.Query()
	status, kind, batch := query.Get("status"), query.Get("kind"), query.Get("batch")
	search := strings.ToLower(strings.TrimSpace(query.Get("q")))
	searchHint := strings.ReplaceAll(strings.ToUpper(search), "-", "")
	codes := slices.DeleteFunc(snapshot.Codes, func(code license.CodeInfo) bool {
		if status != "" && code.Status != status || kind != "" && code.Kind != kind || batch != "" && code.Batch != batch {
			return true
		}
		if search == "" {
			return false
		}
		return !strings.HasPrefix(code.Device, search) && !strings.HasPrefix(code.From, search) &&
			!strings.HasPrefix(code.ID, search) && !strings.Contains(strings.ToLower(code.Batch), search) &&
			!(len(searchHint) <= 4 && code.Hint != "" && strings.HasSuffix(code.Hint, searchHint)) &&
			!(len(searchHint) >= 4 && code.Hint != "" && strings.HasSuffix(searchHint, code.Hint))
	})
	slices.SortFunc(codes, func(x, y license.CodeInfo) int {
		if x.Issued != y.Issued {
			return cmp.Compare(y.Issued, x.Issued)
		}
		return strings.Compare(x.ID, y.ID)
	})
	page, size := pageParams(r)
	return paginate(codes, page, size), nil
}

func (a *adminServer) issueCodes(r *http.Request, _ string) (any, error) {
	var input struct {
		Count int    `json:"count"`
		Note  string `json:"note"`
	}
	if err := decodeBody(r, &input); err != nil {
		return nil, err
	}
	if input.Count < 1 || input.Count > license.MaxIssueBatch {
		return nil, badRequest("请输入 1 至 " + strconv.Itoa(license.MaxIssueBatch) + " 之间的整数")
	}
	if len([]rune(strings.TrimSpace(input.Note))) > license.MaxNoteLength {
		return nil, badRequest("备注最多 " + strconv.Itoa(license.MaxNoteLength) + " 个字符")
	}
	now := a.now()
	batch, codes, err := a.store.IssueLabeled(input.Count, input.Note, now)
	if err != nil {
		return nil, err
	}
	a.audit.record(now, "codes.issue", batch+" ×"+strconv.Itoa(len(codes)), clientAddress(r))
	return map[string]any{"batch": batch, "codes": codes, "created": now.Unix(), "note": strings.TrimSpace(input.Note)}, nil
}

func (a *adminServer) findCode(id string) (license.CodeInfo, error) {
	snapshot, err := a.store.Snapshot()
	if err != nil {
		return license.CodeInfo{}, err
	}
	for _, code := range snapshot.Codes {
		if code.ID == id {
			return code, nil
		}
	}
	return license.CodeInfo{}, license.ErrNotFound
}

func (a *adminServer) lookupCode(r *http.Request, _ string) (any, error) {
	var input struct {
		Code string `json:"code"`
	}
	if err := decodeBody(r, &input); err != nil {
		return nil, err
	}
	id := license.CodeID(input.Code)
	if id == "" {
		return nil, badRequest("激活码格式应为 4 组 8 位十六进制字符")
	}
	code, err := a.findCode(id)
	if errors.Is(err, license.ErrNotFound) {
		return nil, apiError{http.StatusNotFound, "账本中没有这个激活码"}
	}
	return code, err
}

func validID(id string) bool {
	_, err := hex.DecodeString(id)
	return len(id) == 64 && err == nil
}

func (a *adminServer) revokeCode(r *http.Request, _ string) (any, error) {
	var input struct {
		ID string `json:"id"`
	}
	if err := decodeBody(r, &input); err != nil {
		return nil, err
	}
	if !validID(input.ID) {
		return nil, badRequest("记录编号无效")
	}
	now := a.now()
	if err := a.store.Revoke(input.ID, now); err != nil {
		if errors.Is(err, license.ErrNotFound) {
			return nil, err
		}
		return nil, badRequest(err.Error())
	}
	a.audit.record(now, "codes.revoke", input.ID[:12], clientAddress(r))
	return a.findCode(input.ID)
}

// --- Batches ----------------------------------------------------------------

type batchRow struct {
	license.BatchInfo
	Unused   int `json:"unused"`
	Redeemed int `json:"redeemed"`
	Revoked  int `json:"revoked"`
}

func batchRows(snapshot license.Snapshot) []batchRow {
	rows := make([]batchRow, 0, len(snapshot.Batches))
	index := map[string]int{}
	for _, batch := range snapshot.Batches {
		index[batch.ID] = len(rows)
		rows = append(rows, batchRow{BatchInfo: batch})
	}
	for _, code := range snapshot.Codes {
		i, ok := index[code.Batch]
		if !ok {
			continue
		}
		switch code.Status {
		case license.StatusUnused:
			rows[i].Unused++
		case license.StatusRevoked:
			rows[i].Revoked++
		default:
			rows[i].Redeemed++
		}
	}
	slices.SortFunc(rows, func(x, y batchRow) int {
		if x.Created != y.Created {
			return cmp.Compare(y.Created, x.Created)
		}
		return strings.Compare(y.ID, x.ID)
	})
	return rows
}

func (a *adminServer) listBatches(r *http.Request, _ string) (any, error) {
	snapshot, err := a.store.Snapshot()
	if err != nil {
		return nil, err
	}
	rows := batchRows(snapshot)
	if search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q"))); search != "" {
		rows = slices.DeleteFunc(rows, func(row batchRow) bool {
			return !strings.Contains(strings.ToLower(row.ID), search) && !strings.Contains(strings.ToLower(row.Note), search)
		})
	}
	page, size := pageParams(r)
	return paginate(rows, page, size), nil
}

func (a *adminServer) revokeBatch(r *http.Request, _ string) (any, error) {
	var input struct {
		ID string `json:"id"`
	}
	if err := decodeBody(r, &input); err != nil {
		return nil, err
	}
	if input.ID == "" || len(input.ID) > 40 {
		return nil, badRequest("批次编号无效")
	}
	now := a.now()
	changed, err := a.store.RevokeBatch(input.ID, now)
	if err != nil {
		return nil, err
	}
	a.audit.record(now, "batches.revoke", input.ID+" ×"+strconv.Itoa(changed), clientAddress(r))
	return map[string]int{"revoked": changed}, nil
}

// --- Devices ----------------------------------------------------------------

type deviceRow struct {
	Device    string `json:"device"`
	Status    string `json:"status"`
	Expires   int64  `json:"expires,omitempty"`
	Codes     int    `json:"codes"`
	FirstSeen int64  `json:"first_seen,omitempty"`
	LastSeen  int64  `json:"last_seen,omitempty"`
}

func deviceRows(snapshot license.Snapshot, now time.Time) []deviceRow {
	rows := map[string]*deviceRow{}
	get := func(device string) *deviceRow {
		if rows[device] == nil {
			rows[device] = &deviceRow{Device: device, Status: "transferred"}
		}
		return rows[device]
	}
	for device, expires := range snapshot.Expires {
		row := get(device)
		row.Expires = expires
		row.Status = "expired"
		if expires > now.Unix() {
			row.Status = "active"
		}
	}
	for _, code := range snapshot.Codes {
		if code.Device == "" || code.Status == license.StatusPending {
			continue
		}
		row := get(code.Device)
		row.Codes++
		if seen := code.Redeemed; seen != 0 {
			if row.FirstSeen == 0 || seen < row.FirstSeen {
				row.FirstSeen = seen
			}
			row.LastSeen = max(row.LastSeen, seen)
		}
	}
	result := make([]deviceRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, *row)
	}
	slices.SortFunc(result, func(x, y deviceRow) int {
		if x.Expires != y.Expires {
			return cmp.Compare(y.Expires, x.Expires)
		}
		return strings.Compare(x.Device, y.Device)
	})
	return result
}

func (a *adminServer) listDevices(r *http.Request, _ string) (any, error) {
	snapshot, err := a.store.Snapshot()
	if err != nil {
		return nil, err
	}
	status := r.URL.Query().Get("status")
	search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	rows := slices.DeleteFunc(deviceRows(snapshot, a.now()), func(row deviceRow) bool {
		return status != "" && row.Status != status || search != "" && !strings.HasPrefix(row.Device, search)
	})
	page, size := pageParams(r)
	return paginate(rows, page, size), nil
}

func (a *adminServer) transfer(r *http.Request, _ string) (any, error) {
	var input struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := decodeBody(r, &input); err != nil {
		return nil, err
	}
	input.From, input.To = strings.ToLower(strings.TrimSpace(input.From)), strings.ToLower(strings.TrimSpace(input.To))
	if !license.ValidDevice(input.From) || !license.ValidDevice(input.To) {
		return nil, badRequest("设备编号应为 64 位十六进制字符")
	}
	if input.From == input.To {
		return nil, badRequest("原设备和新设备不能相同")
	}
	now := a.now()
	code, err := a.store.Transfer(input.From, input.To, now)
	if err != nil {
		return nil, badRequest("换机需要原设备仍在有效期内，且新设备当前没有有效授权")
	}
	a.audit.record(now, "devices.transfer", input.From[:12]+" → "+input.To[:12], clientAddress(r))
	return map[string]any{"code": code, "device": input.To}, nil
}

func (a *adminServer) renew(r *http.Request, _ string) (any, error) {
	var input struct {
		Device string `json:"device"`
	}
	if err := decodeBody(r, &input); err != nil {
		return nil, err
	}
	input.Device = strings.ToLower(strings.TrimSpace(input.Device))
	if !license.ValidDevice(input.Device) {
		return nil, badRequest("设备编号应为 64 位十六进制字符")
	}
	now := a.now()
	code, expires, err := a.store.Renew(input.Device, now)
	if err != nil {
		return nil, err
	}
	a.audit.record(now, "devices.renew", input.Device[:12], clientAddress(r))
	return map[string]any{"code": code, "device": input.Device, "expires": expires}, nil
}

// --- Audit ------------------------------------------------------------------

func (a *adminServer) listAudit(r *http.Request, _ string) (any, error) {
	entries, err := a.audit.read(500)
	if err != nil {
		return nil, err
	}
	if action := r.URL.Query().Get("action"); action != "" {
		entries = slices.DeleteFunc(entries, func(entry auditEntry) bool { return !strings.HasPrefix(entry.Action, action) })
	}
	page, size := pageParams(r)
	return paginate(entries, page, size), nil
}
