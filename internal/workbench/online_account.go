package workbench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/sylvinus/agent-vm/internal/guestos"
	"github.com/sylvinus/agent-vm/internal/host"
	"github.com/sylvinus/agent-vm/internal/license"
)

// Online CCBox accounts. The account service runs next to the license
// service. The session token it returns is sealed with the local account
// key, so it is only usable while CCBox is unlocked; the local password still
// protects this device's data and works offline.

const onlineSessionPurpose = "online session v1"

var (
	onlineMu          sync.Mutex
	errOnlineDisabled = errors.New("此版本未接入 CCBox 线上账户")
	errOnlineNetwork  = errors.New("无法连接 CCBox 账户服务，请检查网络后重试")
	errOnlineSignedIn = errors.New("线上账户登录已失效，请在「设置 → 账户」重新登录")
)

// onlineError carries the service's message and machine-readable code.
type onlineError struct {
	Status  int
	Code    string
	Message string
}

func (e *onlineError) Error() string { return e.Message }

func onlineCode(err error) string {
	var known *onlineError
	if errors.As(err, &known) {
		return known.Code
	}
	return ""
}

type onlineAccount struct {
	ID         string               `json:"id"`
	Username   string               `json:"username"`
	Expires    int64                `json:"expires,omitempty"`
	Active     bool                 `json:"active"`
	MaxDevices int                  `json:"max_devices"`
	Devices    []license.DeviceInfo `json:"devices"`
}

type onlineRecord struct {
	Version  int    `json:"version"`
	User     string `json:"user"`
	Username string `json:"username"`
	Nonce    []byte `json:"nonce"`
	Sealed   []byte `json:"sealed"`
	// Expires caches the account term from the last reply, for display. The
	// device grant, renewed in the background, is what admits a start.
	Expires int64 `json:"expires,omitempty"`
}

func (s *Server) onlineEnabled() bool { return s.licenseEnabled() && !s.clientOnly && !s.desktopEntry }

func onlineServiceURL(path string) string {
	return strings.TrimSuffix(licenseEndpoint, "/v1/redeem") + path
}

func (s *Server) onlinePath() string { return filepath.Join(s.profile, "online-account.json") }

func onlineDeviceMeta() (string, string) {
	name, _ := os.Hostname()
	if len([]rune(name)) > 64 {
		name = string([]rune(name)[:64])
	}
	return name, runtime.GOOS
}

// onlineCall posts JSON to the account service and decodes the reply.
func onlineCall(ctx context.Context, path, token string, body map[string]string, out any) error {
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, onlineServiceURL(path), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := activationHTTP.Do(req)
	if err != nil {
		return errOnlineNetwork
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return errOnlineNetwork
	}
	if response.StatusCode != http.StatusOK {
		var reply struct {
			Error string `json:"error"`
			Code  string `json:"code"`
		}
		if json.Unmarshal(payload, &reply) != nil || reply.Error == "" {
			return errOnlineNetwork
		}
		return &onlineError{Status: response.StatusCode, Code: reply.Code, Message: reply.Error}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return errOnlineNetwork
	}
	return nil
}

type onlineAuthReply struct {
	Token   string        `json:"token"`
	Account onlineAccount `json:"account"`
}

// onlineAuthenticate registers or signs in this device.
func onlineAuthenticate(ctx context.Context, mode, username, password string) (onlineAuthReply, error) {
	device, err := license.DeviceID()
	if err != nil {
		return onlineAuthReply{}, err
	}
	name, platform := onlineDeviceMeta()
	var reply onlineAuthReply
	err = onlineCall(ctx, "/v1/auth/"+mode, "", map[string]string{"username": strings.TrimSpace(username), "password": password,
		"device": device, "device_name": name, "platform": platform}, &reply)
	if err == nil && (len(reply.Token) != 64 || reply.Account.ID == "") {
		err = errOnlineNetwork
	}
	return reply, err
}

func (s *Server) saveOnlineSession(reply onlineAuthReply) error {
	nonce, sealed, err := s.account.sealSecret(onlineSessionPurpose, []byte(reply.Token))
	if err != nil {
		return err
	}
	return writePrivate(s.onlinePath(), onlineRecord{Version: 1, User: reply.Account.ID, Username: reply.Account.Username, Nonce: nonce, Sealed: sealed, Expires: reply.Account.Expires})
}

// rememberAccountTerm updates the cached account term after a reply.
func (s *Server) rememberAccountTerm(account onlineAccount) {
	record, err := s.loadOnlineRecord()
	if err != nil || record == nil || record.User != account.ID || record.Expires == account.Expires {
		return
	}
	record.Expires = account.Expires
	_ = writePrivate(s.onlinePath(), record)
}

func (s *Server) loadOnlineRecord() (*onlineRecord, error) {
	path := s.onlinePath()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := host.CheckPrivateFile(path); err != nil {
		return nil, err
	}
	var record onlineRecord
	if len(data) > 8<<10 || json.Unmarshal(data, &record) != nil || record.Version != 1 || record.User == "" {
		return nil, errors.New("线上账户记录损坏，请重新登录")
	}
	return &record, nil
}

// onlineSession returns the signed-in account's token, or "" when this
// device is not signed in.
func (s *Server) onlineSession() (*onlineRecord, string, error) {
	record, err := s.loadOnlineRecord()
	if err != nil || record == nil {
		return nil, "", err
	}
	token, err := s.account.openSecret(onlineSessionPurpose, record.Nonce, record.Sealed)
	if err != nil {
		return record, "", err
	}
	return record, string(token), nil
}

func (s *Server) clearOnlineSession() error {
	err := os.Remove(s.onlinePath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// signedInCall runs a request for the signed-in account. A rejected session
// is cleared so the app asks to sign in again.
func (s *Server) signedInCall(ctx context.Context, path string, body map[string]string, out any) error {
	_, token, err := s.onlineSession()
	if err != nil {
		return err
	}
	if token == "" {
		return errOnlineSignedIn
	}
	device, err := license.DeviceID()
	if err != nil {
		return err
	}
	if body == nil {
		body = map[string]string{}
	}
	body["device"] = device
	err = onlineCall(ctx, path, token, body, out)
	if code := onlineCode(err); code == "session" || code == "disabled" {
		_ = s.clearOnlineSession()
	}
	return err
}

// refreshOnlineGrant fetches a device grant for the signed-in account and
// keeps it when it lasts longer than the one already saved.
func (s *Server) refreshOnlineGrant(ctx context.Context) (licenseView, error) {
	if !s.onlineEnabled() {
		return licenseView{}, errOnlineDisabled
	}
	onlineMu.Lock()
	defer onlineMu.Unlock()
	var reply struct {
		Token   string        `json:"token"`
		Account onlineAccount `json:"account"`
	}
	if err := s.signedInCall(ctx, "/v1/account/grant", nil, &reply); err != nil {
		return licenseView{}, err
	}
	s.rememberAccountTerm(reply.Account)
	view, err := s.storeGrant(reply.Token)
	view.AccountExpires = reply.Account.Expires
	return view, err
}

func (s *Server) storeGrant(token string) (licenseView, error) {
	device, err := license.DeviceID()
	if err != nil {
		return licenseView{}, err
	}
	grant, err := verifyLicense(token, device)
	if err != nil || !grant.Active(time.Now()) {
		return licenseView{}, errors.New("授权签名无效，请稍后重试")
	}
	activationMu.Lock()
	defer activationMu.Unlock()
	path := filepath.Join(s.profile, "license.json")
	old, err := loadLicense(path, device)
	if err != nil || old.Expires < grant.Expires {
		if err := writePrivate(path, token); err != nil {
			return licenseView{}, errors.New("无法保存授权，请重试")
		}
	} else {
		grant = old
	}
	return licenseView{State: "active", Device: device, Expires: grant.Expires}, nil
}

// refreshOnlineGrantSoon renews the grant in the background, for example
// right after an unlock. Failures are ignored; the next attempt retries.
func (s *Server) refreshOnlineGrantSoon() {
	if !s.onlineEnabled() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_, _ = s.refreshOnlineGrant(ctx)
	}()
}

// onlineGrantLoop renews the grant every few hours while CCBox runs, so a
// device that is signed in never reaches the end of a short grant.
func (s *Server) onlineGrantLoop(done <-chan struct{}) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			s.refreshOnlineGrantSoon()
		}
	}
}

// redeemOnline applies a code to the signed-in account and saves a new grant.
func (s *Server) redeemOnline(ctx context.Context, code string) (licenseView, error) {
	if err := s.signedInCall(ctx, "/v1/account/redeem", map[string]string{"code": code}, nil); err != nil {
		return licenseView{}, err
	}
	return s.refreshOnlineGrant(ctx)
}

// onlineAccountView is what the settings page shows about the online account.
type onlineAccountView struct {
	Enabled  bool           `json:"enabled"`
	Linked   bool           `json:"linked"`
	Username string         `json:"username,omitempty"`
	Account  *onlineAccount `json:"account,omitempty"`
	Error    string         `json:"error,omitempty"`
}

// onlineAccountRoute serves GET /api/account/online and the signed-in
// actions on it. Callers must have unlocked CCBox in the local window.
func (s *Server) onlineAccountRoute(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if _, ok := s.account.accessFor(s.hasTrustedView(r), false); !ok || !s.hasTrustedView(r) {
		http.Error(w, "请在本机 CCBox 窗口解锁后管理线上账户", http.StatusForbidden)
		return
	}
	view := onlineAccountView{Enabled: s.onlineEnabled()}
	if !view.Enabled {
		jsonReply(w, view)
		return
	}
	if r.Method == http.MethodPost {
		var input struct {
			Action string `json:"action"`
			Target string `json:"target"`
		}
		if !readJSON(w, r, &input, 1024) {
			return
		}
		var err error
		switch input.Action {
		case "remove-device":
			err = s.signedInCall(r.Context(), "/v1/account/devices/remove", map[string]string{"target": input.Target}, nil)
		case "refresh":
			_, err = s.refreshOnlineGrant(r.Context())
			if onlineCode(err) == "no_entitlement" {
				err = nil
			}
		default:
			http.Error(w, "未知操作", http.StatusBadRequest)
			return
		}
		if err != nil {
			requestError(w, err)
			return
		}
	}
	record, err := s.loadOnlineRecord()
	if err != nil {
		requestError(w, err)
		return
	}
	if record == nil {
		jsonReply(w, view)
		return
	}
	view.Linked, view.Username = true, record.Username
	var reply struct {
		Account onlineAccount `json:"account"`
	}
	if err := s.signedInCall(r.Context(), "/v1/account", nil, &reply); err != nil {
		if code := onlineCode(err); code == "session" || code == "disabled" {
			view.Linked = false
		}
		view.Error = err.Error()
	} else {
		view.Account = &reply.Account
		view.Username = reply.Account.Username
		s.rememberAccountTerm(reply.Account)
	}
	jsonReply(w, view)
}

// createWithOnline creates the local account after registering or signing
// in online, so both use the same username and password. Builds without the
// online service keep the local-only account.
func (s *Server) createWithOnline(mode, username, password, avatar string, r *http.Request) error {
	mac := s.macAccountHost()
	if !s.onlineEnabled() {
		if mode == "login" {
			return errOnlineDisabled
		}
		return s.account.createAccount(username, password, avatar, mac)
	}
	if s.account.exists() {
		return errors.New("此配置已创建本地账户，请解锁后在「设置 → 账户」登录线上账户")
	}
	name := strings.TrimSpace(username)
	if err := validAccountPassword(password); err != nil {
		return err
	}
	if mode == "register" {
		if err := validAccountName(name); err != nil {
			return err
		}
	}
	if mac {
		// Check the macOS password rule before the service sees the request,
		// so a successful sign-in is never followed by a local failure.
		if err := validMacAccount(macLocalName(name), password); err != nil {
			return err
		}
	}
	reply, err := onlineAuthenticate(r.Context(), mode, username, password)
	if err != nil {
		return err
	}
	name = reply.Account.Username
	if mac {
		name = macLocalName(name)
	}
	if err := s.account.createAccount(name, password, avatar, mac); err != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = onlineCall(ctx, "/v1/auth/logout", reply.Token, map[string]string{}, nil)
		return err
	}
	// If saving the session fails the local account still exists; signing in
	// again from settings recovers it.
	_ = s.saveOnlineSession(reply)
	return nil
}

// changeOnlinePassword keeps the online password in step with the local
// one. It runs first, so a network failure changes neither. An account whose
// online password already differs keeps it.
func (s *Server) changeOnlinePassword(ctx context.Context, oldPassword, newPassword string) error {
	if !s.onlineEnabled() {
		return nil
	}
	if err := s.account.checkPassword(oldPassword); err != nil {
		return err
	}
	if err := validAccountPassword(newPassword); err != nil {
		return err
	}
	_, token, err := s.onlineSession()
	if err != nil || token == "" {
		return nil
	}
	err = s.signedInCall(ctx, "/v1/account/password", map[string]string{"old_password": oldPassword, "password": newPassword}, nil)
	switch code := onlineCode(err); {
	case err == nil, code == "credentials", code == "session", code == "disabled":
		return nil
	default:
		return err
	}
}

// macLocalName is the local account name for an online account on a Mac.
// There the local name is also the shared macOS login, which allows only
// lowercase letters, digits and underscores. Online names are case-insensitive
// and may also contain hyphens and dots, so map them onto that set.
func macLocalName(online string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(online)) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	local := b.String()
	if local == "" || local[0] < 'a' || local[0] > 'z' {
		local = "u" + local
	}
	local = local[:min(len(local), 31)]
	if !guestos.ValidMacOSAccountName(local) {
		local = "cage_" + local
		local = local[:min(len(local), 31)]
	}
	return local
}
