package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sylvinus/agent-vm/internal/license"
)

// Public account endpoints used by the desktop app. Every request is a JSON
// POST; signed-in requests carry the session token as a bearer token and the
// device ID in the body, and a token only works from the device it was
// issued to.

type rateLimiter struct {
	mu     sync.Mutex
	events map[string][]time.Time
}

type rateBudget struct {
	key    string
	limit  int
	window time.Duration
}

// allow records one event for key and reports whether it stays within limit
// events per window.
func (l *rateLimiter) allow(key string, limit int, window time.Duration, now time.Time) bool {
	return l.reserve(now, rateBudget{key, limit, window}) == 0
}

// reserve consumes every budget together or leaves them all unspent.
func (l *rateLimiter) reserve(now time.Time, budgets ...rateBudget) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.events == nil {
		l.events = map[string][]time.Time{}
	}
	if len(l.events) > 50000 {
		for k, times := range l.events {
			if len(times) == 0 || now.Sub(times[len(times)-1]) > time.Hour {
				delete(l.events, k)
			}
		}
	}
	var retry time.Duration
	for _, budget := range budgets {
		recent := slices.DeleteFunc(l.events[budget.key], func(at time.Time) bool { return now.Sub(at) >= budget.window })
		if len(recent) == 0 {
			delete(l.events, budget.key)
		} else {
			l.events[budget.key] = recent
		}
		if len(recent) >= budget.limit {
			oldest := slices.MinFunc(recent, time.Time.Compare)
			retry = max(retry, oldest.Add(budget.window).Sub(now))
		}
	}
	if retry > 0 {
		return retry
	}
	for _, budget := range budgets {
		l.events[budget.key] = append(l.events[budget.key], now)
	}
	return 0
}

// count reports events for key within window without recording one.
func (l *rateLimiter) count(key string, window time.Duration, now time.Time) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, at := range l.events[key] {
		if now.Sub(at) < window {
			n++
		}
	}
	return n
}

type accountAPI struct {
	store             *license.Store
	limiter           rateLimiter
	now               func() time.Time
	registrationSlots chan struct{}
}

type accountRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	OldPassword string `json:"old_password"`
	Device      string `json:"device"`
	DeviceName  string `json:"device_name"`
	Platform    string `json:"platform"`
	Code        string `json:"code"`
	Target      string `json:"target"`
}

func registerAccountAPI(mux *http.ServeMux, store *license.Store) {
	a := newAccountAPI(store, time.Now)
	mux.HandleFunc("POST /v1/auth/register", a.handle(false, a.register))
	mux.HandleFunc("POST /v1/auth/login", a.handle(false, a.login))
	mux.HandleFunc("POST /v1/auth/logout", a.handle(false, a.logout))
	mux.HandleFunc("POST /v1/auth/reset", a.handle(false, a.reset))
	mux.HandleFunc("POST /v1/account", a.handle(true, a.account))
	mux.HandleFunc("POST /v1/account/grant", a.handle(true, a.grant))
	mux.HandleFunc("POST /v1/account/redeem", a.handle(true, a.redeem))
	mux.HandleFunc("POST /v1/account/devices/remove", a.handle(true, a.removeDevice))
	mux.HandleFunc("POST /v1/account/password", a.handle(true, a.password))
}

func newAccountAPI(store *license.Store, now func() time.Time) *accountAPI {
	return &accountAPI{store: store, now: now, registrationSlots: make(chan struct{}, 4)}
}

type publicError struct {
	status     int
	code       string
	err        error
	retryAfter int
}

func (e publicError) Error() string { return e.err.Error() }

func reject(status int, code, message string) error {
	return publicError{status: status, code: code, err: errors.New(message)}
}

func (a *accountAPI) handle(signedIn bool, handler func(*http.Request, accountRequest, string) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		var input accountRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
			writePublicError(w, reject(http.StatusBadRequest, "invalid_request", "请求内容无效"))
			return
		}
		input.Device = strings.ToLower(strings.TrimSpace(input.Device))
		token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if signedIn && (token == "" || !license.ValidDevice(input.Device)) {
			writePublicError(w, license.ErrSession)
			return
		}
		result, err := handler(r, input, strings.TrimSpace(token))
		if err != nil {
			writePublicError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func writePublicError(w http.ResponseWriter, err error) {
	known := []struct {
		err    error
		status int
		code   string
	}{
		{license.ErrCredentials, http.StatusUnauthorized, "credentials"},
		{license.ErrSession, http.StatusUnauthorized, "session"},
		{license.ErrAccountOff, http.StatusForbidden, "disabled"},
		{license.ErrNoEntitlement, http.StatusForbidden, "no_entitlement"},
		{license.ErrUsernameTaken, http.StatusConflict, "username_taken"},
		{license.ErrDeviceLimit, http.StatusConflict, "device_limit"},
		{license.ErrUsed, http.StatusConflict, "code_used"},
		{license.ErrRedeemedHere, http.StatusConflict, "code_used"},
		{license.ErrInvalid, http.StatusBadRequest, "code_invalid"},
		{license.ErrRevoked, http.StatusBadRequest, "code_revoked"},
		{license.ErrResetInvalid, http.StatusBadRequest, "reset_invalid"},
		{license.ErrNotFound, http.StatusNotFound, "not_found"},
		{license.ErrInvalidAccount, http.StatusBadRequest, "invalid_request"},
	}
	var public publicError
	if errors.As(err, &public) {
		if public.retryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(public.retryAfter))
		}
		writeJSON(w, public.status, map[string]string{"error": public.Error(), "code": public.code})
		return
	}
	for _, item := range known {
		if errors.Is(err, item.err) {
			writeJSON(w, item.status, map[string]string{"error": err.Error(), "code": item.code})
			return
		}
	}
	log.Printf("account service failed: %v", err)
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "账户服务暂时不可用，请稍后重试", "code": "unavailable"})
}

func tooMany() error {
	return reject(http.StatusTooManyRequests, "rate_limited", "尝试次数过多，请稍后再试")
}

func validCredentials(input accountRequest) error {
	if err := license.ValidUsername(input.Username); err != nil {
		return reject(http.StatusBadRequest, "invalid_username", err.Error())
	}
	if err := license.ValidPassword(input.Password); err != nil {
		return reject(http.StatusBadRequest, "invalid_password", err.Error())
	}
	if !license.ValidDevice(input.Device) {
		return reject(http.StatusBadRequest, "invalid_request", "设备编号无效")
	}
	return nil
}

const (
	registrationWindow      = 10 * time.Minute
	registrationsPerDevice  = 6
	registrationsPerNetwork = 60
)

func registrationCooldown(wait time.Duration) error {
	seconds := int((wait + time.Second - 1) / time.Second)
	duration := strconv.Itoa(seconds) + " 秒"
	if seconds >= 60 {
		duration = strconv.Itoa((seconds+59)/60) + " 分钟"
	}
	return publicError{
		status:     http.StatusTooManyRequests,
		code:       "rate_limited",
		err:        errors.New("注册请求过于频繁，请在 " + duration + "后重试。已有账户可直接登录。"),
		retryAfter: seconds,
	}
}

func (a *accountAPI) register(r *http.Request, input accountRequest, _ string) (any, error) {
	if err := validCredentials(input); err != nil {
		return nil, err
	}
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case a.registrationSlots <- struct{}{}:
		defer func() { <-a.registrationSlots }()
	case <-r.Context().Done():
		return nil, r.Context().Err()
	case <-timer.C:
		return nil, publicError{
			status: http.StatusServiceUnavailable, code: "unavailable",
			err: errors.New("注册服务繁忙，请在 2 秒后重试。"), retryAfter: 2,
		}
	}
	now := a.now()
	if wait := a.limiter.reserve(now,
		rateBudget{"register-device:" + input.Device, registrationsPerDevice, registrationWindow},
		rateBudget{"register-network:" + clientAddress(r), registrationsPerNetwork, registrationWindow},
	); wait > 0 {
		return nil, registrationCooldown(wait)
	}
	token, info, err := a.store.Register(input.Username, input.Password, input.Device, license.DeviceMeta{Name: input.DeviceName, Platform: input.Platform}, now)
	if err != nil {
		return nil, err
	}
	log.Printf("account registered: %s", license.ShortID(info.ID))
	return map[string]any{"token": token, "account": info}, nil
}

const (
	loginWindowPublic = 15 * time.Minute
	maxAddressLogins  = 30
	maxNameFailures   = 10
)

func (a *accountAPI) login(r *http.Request, input accountRequest, _ string) (any, error) {
	if !license.ValidDevice(input.Device) || input.Username == "" || len(input.Username) > 256 {
		return nil, license.ErrCredentials
	}
	now := a.now()
	nameKey := "login-name:" + strings.ToLower(strings.TrimSpace(input.Username))
	if !a.limiter.allow("login:"+clientAddress(r), maxAddressLogins, loginWindowPublic, now) ||
		a.limiter.count(nameKey, loginWindowPublic, now) >= maxNameFailures {
		return nil, tooMany()
	}
	token, info, err := a.store.Login(input.Username, input.Password, input.Device, license.DeviceMeta{Name: input.DeviceName, Platform: input.Platform}, now)
	if errors.Is(err, license.ErrCredentials) {
		a.limiter.allow(nameKey, maxNameFailures+1, loginWindowPublic, now)
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"token": token, "account": info}, nil
}

func (a *accountAPI) logout(_ *http.Request, _ accountRequest, token string) (any, error) {
	if token != "" {
		if err := a.store.Logout(token); err != nil {
			return nil, err
		}
	}
	return map[string]bool{"ok": true}, nil
}

func (a *accountAPI) reset(r *http.Request, input accountRequest, _ string) (any, error) {
	if err := license.ValidPassword(input.Password); err != nil {
		return nil, reject(http.StatusBadRequest, "invalid_password", err.Error())
	}
	if len(input.Code) > 32 || len(input.Username) > 256 {
		return nil, license.ErrResetInvalid
	}
	if !a.limiter.allow("reset:"+clientAddress(r), 10, time.Hour, a.now()) {
		return nil, tooMany()
	}
	if err := a.store.ResetPassword(input.Username, input.Code, input.Password, a.now()); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}

func (a *accountAPI) account(_ *http.Request, input accountRequest, token string) (any, error) {
	info, err := a.store.Account(token, input.Device, a.now())
	if err != nil {
		return nil, err
	}
	return map[string]any{"account": info}, nil
}

func (a *accountAPI) grant(_ *http.Request, input accountRequest, token string) (any, error) {
	signed, info, err := a.store.AccountGrant(token, input.Device, a.now())
	if errors.Is(err, license.ErrNoEntitlement) {
		return nil, publicError{status: http.StatusForbidden, code: "no_entitlement", err: err}
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"token": signed, "account": info}, nil
}

func (a *accountAPI) redeem(r *http.Request, input accountRequest, token string) (any, error) {
	if len(input.Code) == 0 || len(input.Code) > 64 {
		return nil, license.ErrInvalid
	}
	if !a.limiter.allow("redeem:"+clientAddress(r), 30, time.Hour, a.now()) {
		return nil, tooMany()
	}
	info, err := a.store.RedeemToAccount(token, input.Device, input.Code, a.now())
	if err != nil {
		return nil, err
	}
	return map[string]any{"account": info}, nil
}

func (a *accountAPI) removeDevice(_ *http.Request, input accountRequest, token string) (any, error) {
	target := strings.ToLower(strings.TrimSpace(input.Target))
	if !license.ValidDevice(target) {
		return nil, reject(http.StatusBadRequest, "invalid_request", "设备编号无效")
	}
	if target == input.Device {
		return nil, reject(http.StatusBadRequest, "current_device", "不能移除当前设备，请使用退出登录")
	}
	info, err := a.store.RemoveDevice(token, input.Device, target, a.now())
	if err != nil {
		return nil, err
	}
	return map[string]any{"account": info}, nil
}

func (a *accountAPI) password(_ *http.Request, input accountRequest, token string) (any, error) {
	if err := license.ValidPassword(input.Password); err != nil {
		return nil, reject(http.StatusBadRequest, "invalid_password", err.Error())
	}
	if err := a.store.ChangePassword(token, input.Device, input.OldPassword, input.Password, a.now()); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}

// maxDevicesFromEnv reads CCBOX_LICENSE_MAX_DEVICES (1–20).
func maxDevicesFromEnv(value string) (int, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 1 || n > 20 {
		return 0, errors.New("CCBOX_LICENSE_MAX_DEVICES must be an integer from 1 to 20")
	}
	return n, nil
}
