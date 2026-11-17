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

// allow records one event for key and reports whether it stays within limit
// events per window.
func (l *rateLimiter) allow(key string, limit int, window time.Duration, now time.Time) bool {
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
	recent := slices.DeleteFunc(l.events[key], func(at time.Time) bool { return now.Sub(at) > window })
	if len(recent) >= limit {
		l.events[key] = recent
		return false
	}
	l.events[key] = append(recent, now)
	return true
}

// count reports events for key within window without recording one.
func (l *rateLimiter) count(key string, window time.Duration, now time.Time) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, at := range l.events[key] {
		if now.Sub(at) <= window {
			n++
		}
	}
	return n
}

type accountAPI struct {
	store   *license.Store
	limiter rateLimiter
	now     func() time.Time
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
	a := &accountAPI{store: store, now: time.Now}
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

type publicError struct {
	status int
	code   string
	err    error
}

func (e publicError) Error() string { return e.err.Error() }

func reject(status int, code, message string) error {
	return publicError{status, code, errors.New(message)}
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
		{license.ErrInvalid, http.StatusBadRequest, "code_invalid"},
		{license.ErrRevoked, http.StatusBadRequest, "code_revoked"},
		{license.ErrResetInvalid, http.StatusBadRequest, "reset_invalid"},
		{license.ErrNotFound, http.StatusNotFound, "not_found"},
		{license.ErrInvalidAccount, http.StatusBadRequest, "invalid_request"},
	}
	var public publicError
	if errors.As(err, &public) {
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

func (a *accountAPI) register(r *http.Request, input accountRequest, _ string) (any, error) {
	if err := validCredentials(input); err != nil {
		return nil, err
	}
	now := a.now()
	if !a.limiter.allow("register:"+clientAddress(r), 5, time.Hour, now) {
		return nil, tooMany()
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
		return nil, publicError{http.StatusForbidden, "no_entitlement", err}
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
