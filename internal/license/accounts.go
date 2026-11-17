package license

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Online accounts live in the same ledger as activation codes, so redeeming a
// code into an account is one atomic write. Passwords are stored only as
// argon2id hashes; session tokens only as SHA-256 digests.

var (
	ErrUsernameTaken  = errors.New("用户名已被注册")
	ErrCredentials    = errors.New("用户名或密码不正确")
	ErrAccountOff     = errors.New("此账户已停用，请联系客服")
	ErrDeviceLimit    = errors.New("此账户登录的设备已达上限，请先在其他设备退出登录，或联系客服解绑")
	ErrSession        = errors.New("登录已失效，请重新登录")
	ErrNoEntitlement  = errors.New("账户没有有效授权，请兑换激活码")
	ErrResetInvalid   = errors.New("重置码无效或已过期，请联系客服重新获取")
	ErrInvalidAccount = errors.New("账户信息无效")
)

const (
	// AccountGrantTerm bounds how long one device grant lasts. Clients renew
	// it in the background, so disabling an account or removing a device
	// takes effect within this time even for a device that stays offline.
	AccountGrantTerm = 7 * 24 * time.Hour
	sessionLifetime  = 90 * 24 * time.Hour
	resetLifetime    = 24 * time.Hour
	maxEvents        = 200
	// DefaultMaxDevices applies when the store does not set a limit.
	DefaultMaxDevices = 2
)

// Entitlement event sources.
const (
	SourceCode   = "code"
	SourceLegacy = "legacy"
	SourceAdmin  = "admin"
)

type userRecord struct {
	Username       string                `json:"username"`
	Password       string                `json:"password"`
	Created        int64                 `json:"created"`
	LastLogin      int64                 `json:"last_login,omitempty"`
	Disabled       int64                 `json:"disabled,omitempty"`
	DisabledReason string                `json:"disabled_reason,omitempty"`
	Expires        int64                 `json:"expires,omitempty"`
	Devices        map[string]userDevice `json:"devices,omitempty"`
	Events         []EntitlementEvent    `json:"events,omitempty"`
	Reset          *resetRecord          `json:"reset,omitempty"`
}

type userDevice struct {
	Name     string `json:"name,omitempty"`
	Platform string `json:"platform,omitempty"`
	Added    int64  `json:"added"`
	Seen     int64  `json:"seen,omitempty"`
}

type resetRecord struct {
	Digest  string `json:"digest"`
	Expires int64  `json:"expires"`
}

type sessionRecord struct {
	User    string `json:"user"`
	Device  string `json:"device"`
	Created int64  `json:"created"`
	Seen    int64  `json:"seen"`
}

// EntitlementEvent records one change to an account's term.
type EntitlementEvent struct {
	Time    int64  `json:"time"`
	Source  string `json:"source"`
	Seconds int64  `json:"seconds,omitempty"`
	Expires int64  `json:"expires"`
	Ref     string `json:"ref,omitempty"`
	Note    string `json:"note,omitempty"`
}

// DeviceInfo describes a device signed in to an account.
type DeviceInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	Platform string `json:"platform,omitempty"`
	Added    int64  `json:"added"`
	Seen     int64  `json:"seen,omitempty"`
	Current  bool   `json:"current,omitempty"`
}

// AccountInfo is what a signed-in client sees about its own account.
type AccountInfo struct {
	ID         string       `json:"id"`
	Username   string       `json:"username"`
	Expires    int64        `json:"expires,omitempty"`
	Active     bool         `json:"active"`
	MaxDevices int          `json:"max_devices"`
	Devices    []DeviceInfo `json:"devices"`
}

// DeviceMeta is optional, user-visible description of a signing-in device.
type DeviceMeta struct {
	Name     string
	Platform string
}

func (m DeviceMeta) clean() DeviceMeta {
	trim := func(value string, limit int) string {
		value = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return -1
			}
			return r
		}, strings.TrimSpace(value))
		if utf8.RuneCountInString(value) > limit {
			value = string([]rune(value)[:limit])
		}
		return value
	}
	return DeviceMeta{Name: trim(m.Name, 64), Platform: trim(m.Platform, 32)}
}

// ValidUsername accepts 3–32 letters, digits, "_", "-" or ".", including
// non-Latin letters. Names compare case-insensitively.
func ValidUsername(name string) error {
	name = strings.TrimSpace(name)
	count := utf8.RuneCountInString(name)
	if count < 3 || count > 32 {
		return errors.New("用户名需要 3–32 个字符")
	}
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' && r != '.' {
			return errors.New("用户名只能包含字母、数字、下划线、连字符和点")
		}
	}
	return nil
}

// ValidPassword matches the desktop's local password rule.
func ValidPassword(password string) error {
	if utf8.RuneCountInString(password) < 8 || len(password) > 1024 || strings.ContainsRune(password, 0) {
		return errors.New("密码至少 8 个字符，最多 1024 字节")
	}
	return nil
}

func normalizeName(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// Password hashing uses argon2id with the OWASP minimum memory setting so a
// small server can absorb login traffic.
const (
	hashTime    = 2
	hashMemory  = 19 * 1024
	hashThreads = 1
)

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	sum := argon2.IDKey([]byte(password), salt, hashTime, hashMemory, hashThreads, 32)
	return "argon2id$" + encoding.EncodeToString(salt) + "$" + encoding.EncodeToString(sum), nil
}

func checkPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 3 || parts[0] != "argon2id" || len(password) > 1024 {
		return false
	}
	salt, err1 := encoding.DecodeString(parts[1])
	want, err2 := encoding.DecodeString(parts[2])
	if err1 != nil || err2 != nil || len(want) != 32 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, hashTime, hashMemory, hashThreads, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}

// dummyHash keeps a failed lookup as slow as a wrong password.
var dummyHash, _ = hashPassword("ccbox-unused-password")

func randomHex(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func tokenDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func ensureAccounts(state *ledger) {
	if state.Users == nil {
		state.Users = map[string]userRecord{}
	}
	if state.Names == nil {
		state.Names = map[string]string{}
	}
	if state.Sessions == nil {
		state.Sessions = map[string]sessionRecord{}
	}
	if state.Owners == nil {
		state.Owners = map[string]string{}
	}
}

func (user *userRecord) extend(now time.Time, seconds int64, source, ref, note string) {
	if seconds > 0 {
		user.Expires = max(now.Unix(), user.Expires) + seconds
	} else {
		user.Expires = max(user.Expires+seconds, now.Unix())
	}
	user.record(EntitlementEvent{Time: now.Unix(), Source: source, Seconds: seconds, Expires: user.Expires, Ref: ref, Note: note})
}

func (user *userRecord) raiseTo(now time.Time, expires int64, source, ref string) {
	if expires <= user.Expires || expires <= now.Unix() {
		return
	}
	seconds := expires - max(now.Unix(), user.Expires)
	user.Expires = expires
	user.record(EntitlementEvent{Time: now.Unix(), Source: source, Seconds: seconds, Expires: expires, Ref: ref})
}

func (user *userRecord) record(event EntitlementEvent) {
	user.Events = append(user.Events, event)
	if len(user.Events) > maxEvents {
		user.Events = slices.Clone(user.Events[len(user.Events)-maxEvents:])
	}
}

func (s *Store) maxDevices() int {
	if s.MaxDevices > 0 {
		return s.MaxDevices
	}
	return DefaultMaxDevices
}

// attach signs a device in to an account: it enforces the device limit,
// moves the device's legacy activation term into the account once, and
// opens a session. The caller holds the ledger lock.
func (s *Store) attach(state *ledger, id string, user *userRecord, device string, meta DeviceMeta, now time.Time) (string, error) {
	if user.Devices == nil {
		user.Devices = map[string]userDevice{}
	}
	entry, known := user.Devices[device]
	if !known && len(user.Devices) >= s.maxDevices() {
		return "", ErrDeviceLimit
	}
	meta = meta.clean()
	if !known {
		entry = userDevice{Added: now.Unix()}
	}
	if meta.Name != "" {
		entry.Name = meta.Name
	}
	if meta.Platform != "" {
		entry.Platform = meta.Platform
	}
	entry.Seen = now.Unix()
	user.Devices[device] = entry
	if owner := state.Owners[device]; owner == "" {
		state.Owners[device] = id
		user.raiseTo(now, state.Expires[device], SourceLegacy, device[:12])
	}
	user.LastLogin = now.Unix()
	token, err := randomHex(32)
	if err != nil {
		return "", err
	}
	state.Sessions[tokenDigest(token)] = sessionRecord{User: id, Device: device, Created: now.Unix(), Seen: now.Unix()}
	return token, nil
}

func (s *Store) accountInfo(id string, user userRecord, device string, now time.Time) AccountInfo {
	info := AccountInfo{ID: id, Username: user.Username, Expires: user.Expires, Active: user.Expires > now.Unix(), MaxDevices: s.maxDevices(), Devices: []DeviceInfo{}}
	for key, entry := range user.Devices {
		info.Devices = append(info.Devices, DeviceInfo{ID: key, Name: entry.Name, Platform: entry.Platform, Added: entry.Added, Seen: entry.Seen, Current: key == device})
	}
	slices.SortFunc(info.Devices, func(a, b DeviceInfo) int { return int(a.Added - b.Added) })
	return info
}

// Register creates an account and signs the device in.
func (s *Store) Register(username, password, device string, meta DeviceMeta, now time.Time) (string, AccountInfo, error) {
	username = strings.TrimSpace(username)
	if err := ValidUsername(username); err != nil {
		return "", AccountInfo{}, err
	}
	if err := ValidPassword(password); err != nil {
		return "", AccountInfo{}, err
	}
	if !ValidDevice(device) {
		return "", AccountInfo{}, ErrInvalidAccount
	}
	hash, err := hashPassword(password)
	if err != nil {
		return "", AccountInfo{}, err
	}
	var token string
	var info AccountInfo
	err = s.update(func(state *ledger) (bool, error) {
		ensureAccounts(state)
		key := normalizeName(username)
		if _, taken := state.Names[key]; taken {
			return false, ErrUsernameTaken
		}
		id, err := randomHex(16)
		if err != nil {
			return false, err
		}
		user := userRecord{Username: username, Password: hash, Created: now.Unix()}
		if token, err = s.attach(state, id, &user, device, meta, now); err != nil {
			return false, err
		}
		state.Users[id] = user
		state.Names[key] = id
		info = s.accountInfo(id, user, device, now)
		return true, nil
	})
	return token, info, err
}

// Login checks a username and password and signs the device in. The slow
// password check runs outside the ledger lock.
func (s *Store) Login(username, password, device string, meta DeviceMeta, now time.Time) (string, AccountInfo, error) {
	if !ValidDevice(device) {
		return "", AccountInfo{}, ErrInvalidAccount
	}
	var id, hash string
	if err := s.read(func(state *ledger) {
		id = state.Names[normalizeName(username)]
		hash = state.Users[id].Password
	}); err != nil {
		return "", AccountInfo{}, err
	}
	if id == "" {
		checkPassword(dummyHash, password)
		return "", AccountInfo{}, ErrCredentials
	}
	if !checkPassword(hash, password) {
		return "", AccountInfo{}, ErrCredentials
	}
	var token string
	var info AccountInfo
	err := s.update(func(state *ledger) (bool, error) {
		ensureAccounts(state)
		user, ok := state.Users[id]
		// The password may have changed while it was being checked.
		if !ok || user.Password != hash {
			return false, ErrCredentials
		}
		if user.Disabled != 0 {
			return false, ErrAccountOff
		}
		var err error
		if token, err = s.attach(state, id, &user, device, meta, now); err != nil {
			return false, err
		}
		state.Users[id] = user
		info = s.accountInfo(id, user, device, now)
		return true, nil
	})
	return token, info, err
}

// session resolves a token for a device, refreshing its last use at most
// once an hour so ordinary calls do not rewrite the ledger.
func (s *Store) session(state *ledger, token, device string, now time.Time) (string, userRecord, bool, error) {
	ensureAccounts(state)
	digest := tokenDigest(token)
	current, ok := state.Sessions[digest]
	if !ok || current.Device != device {
		return "", userRecord{}, false, ErrSession
	}
	if now.Unix()-current.Seen > int64(sessionLifetime/time.Second) {
		delete(state.Sessions, digest)
		return "", userRecord{}, true, ErrSession
	}
	user, ok := state.Users[current.User]
	if !ok {
		delete(state.Sessions, digest)
		return "", userRecord{}, true, ErrSession
	}
	if user.Disabled != 0 {
		return "", userRecord{}, false, ErrAccountOff
	}
	if _, signedIn := user.Devices[device]; !signedIn {
		delete(state.Sessions, digest)
		return "", userRecord{}, true, ErrSession
	}
	changed := false
	if now.Unix()-current.Seen >= 3600 {
		current.Seen = now.Unix()
		state.Sessions[digest] = current
		entry := user.Devices[device]
		entry.Seen = now.Unix()
		user.Devices[device] = entry
		state.Users[current.User] = user
		changed = true
	}
	return current.User, user, changed, nil
}

// withSession runs change for the signed-in account. A failed session check
// may still save cleanup of an expired session.
func (s *Store) withSession(token, device string, now time.Time, change func(state *ledger, id string, user *userRecord) (bool, error)) error {
	if !ValidDevice(device) {
		return ErrSession
	}
	var failure error
	err := s.update(func(state *ledger) (bool, error) {
		id, user, touched, err := s.session(state, token, device, now)
		if err != nil {
			failure = err
			return touched, nil
		}
		changed, err := change(state, id, &user)
		if err != nil {
			return false, err
		}
		state.Users[id] = user
		return changed || touched, nil
	})
	if err != nil {
		return err
	}
	return failure
}

// Account returns the signed-in account.
func (s *Store) Account(token, device string, now time.Time) (AccountInfo, error) {
	var info AccountInfo
	err := s.withSession(token, device, now, func(_ *ledger, id string, user *userRecord) (bool, error) {
		info = s.accountInfo(id, *user, device, now)
		return false, nil
	})
	return info, err
}

// AccountGrant signs a device grant for the signed-in account, valid until
// the account term ends or for AccountGrantTerm, whichever comes first.
func (s *Store) AccountGrant(token, device string, now time.Time) (string, AccountInfo, error) {
	var signed string
	var info AccountInfo
	err := s.withSession(token, device, now, func(_ *ledger, id string, user *userRecord) (bool, error) {
		info = s.accountInfo(id, *user, device, now)
		if user.Expires <= now.Unix() {
			return false, ErrNoEntitlement
		}
		expires := min(user.Expires, now.Add(AccountGrantTerm).Unix())
		signed = Grant{Version: 1, Device: device, Expires: expires, Account: id}.sign(s.key)
		return false, nil
	})
	return signed, info, err
}

// RedeemToAccount applies an activation code to the signed-in account.
func (s *Store) RedeemToAccount(token, device, code string, now time.Time) (AccountInfo, error) {
	digest := codeDigest(code)
	if digest == "" {
		return AccountInfo{}, ErrInvalid
	}
	var info AccountInfo
	err := s.withSession(token, device, now, func(state *ledger, id string, user *userRecord) (bool, error) {
		record, ok := state.Codes[digest]
		if !ok {
			return false, ErrInvalid
		}
		if record.Revoked != 0 {
			return false, ErrRevoked
		}
		kind := record.Kind
		if kind == "" {
			kind = KindCode
		}
		switch {
		case kind == KindCode && record.Device == "":
			user.extend(now, int64(Duration/time.Second), SourceCode, digest[:12], "")
		case kind != KindCode && record.Device == device && record.Redeemed == 0:
			// A support code bound to this device carries its term over.
			user.raiseTo(now, record.Expires, SourceCode, digest[:12])
		default:
			return false, ErrUsed
		}
		record.Device, record.Account, record.Redeemed, record.Expires = device, id, now.Unix(), user.Expires
		state.Codes[digest] = record
		info = s.accountInfo(id, *user, device, now)
		return true, nil
	})
	return info, err
}

// Logout ends the session. An unknown token is not an error.
func (s *Store) Logout(token string) error {
	return s.update(func(state *ledger) (bool, error) {
		digest := tokenDigest(token)
		if _, ok := state.Sessions[digest]; !ok {
			return false, nil
		}
		delete(state.Sessions, digest)
		return true, nil
	})
}

func removeDevice(state *ledger, id string, user *userRecord, device string) bool {
	if _, ok := user.Devices[device]; !ok {
		return false
	}
	delete(user.Devices, device)
	for digest, current := range state.Sessions {
		if current.User == id && current.Device == device {
			delete(state.Sessions, digest)
		}
	}
	return true
}

// RemoveDevice signs one of the account's devices out from another device.
func (s *Store) RemoveDevice(token, device, target string, now time.Time) (AccountInfo, error) {
	var info AccountInfo
	err := s.withSession(token, device, now, func(state *ledger, id string, user *userRecord) (bool, error) {
		if target == device {
			return false, errors.New("不能移除当前设备，请使用退出登录")
		}
		if !removeDevice(state, id, user, target) {
			return false, ErrNotFound
		}
		info = s.accountInfo(id, *user, device, now)
		return true, nil
	})
	return info, err
}

// ChangePassword changes the password and signs out every other device.
func (s *Store) ChangePassword(token, device, oldPassword, newPassword string, now time.Time) error {
	if err := ValidPassword(newPassword); err != nil {
		return err
	}
	var hash string
	if err := s.withSession(token, device, now, func(_ *ledger, _ string, user *userRecord) (bool, error) {
		hash = user.Password
		return false, nil
	}); err != nil {
		return err
	}
	if !checkPassword(hash, oldPassword) {
		return ErrCredentials
	}
	next, err := hashPassword(newPassword)
	if err != nil {
		return err
	}
	return s.withSession(token, device, now, func(state *ledger, id string, user *userRecord) (bool, error) {
		if user.Password != hash {
			return false, ErrCredentials
		}
		user.Password, user.Reset = next, nil
		current := tokenDigest(token)
		for digest, session := range state.Sessions {
			if session.User == id && digest != current {
				delete(state.Sessions, digest)
			}
		}
		return true, nil
	})
}

// ResetPassword sets a new password with a one-time code issued by support,
// and signs out every device.
func (s *Store) ResetPassword(username, code, password string, now time.Time) error {
	if err := ValidPassword(password); err != nil {
		return err
	}
	next, err := hashPassword(password)
	if err != nil {
		return err
	}
	digest := tokenDigest(strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", "")))
	return s.update(func(state *ledger) (bool, error) {
		ensureAccounts(state)
		id := state.Names[normalizeName(username)]
		user, ok := state.Users[id]
		if !ok || user.Reset == nil || user.Reset.Expires <= now.Unix() ||
			subtle.ConstantTimeCompare([]byte(user.Reset.Digest), []byte(digest)) != 1 {
			return false, ErrResetInvalid
		}
		user.Password, user.Reset = next, nil
		state.Users[id] = user
		for key, session := range state.Sessions {
			if session.User == id {
				delete(state.Sessions, key)
			}
		}
		return true, nil
	})
}

// read gives a consistent view of the ledger without writing it.
func (s *Store) read(view func(*ledger)) error {
	return s.update(func(state *ledger) (bool, error) {
		ensureAccounts(state)
		view(state)
		return false, nil
	})
}

// --- Administration ---------------------------------------------------------

// User statuses shown to administrators.
const (
	UserActive   = "active"
	UserExpired  = "expired"
	UserNone     = "none"
	UserDisabled = "disabled"
)

// UserInfo is the administrator view of an account. It never includes the
// password hash, session tokens or reset codes.
type UserInfo struct {
	ID             string             `json:"id"`
	Username       string             `json:"username"`
	Status         string             `json:"status"`
	Created        int64              `json:"created"`
	LastLogin      int64              `json:"last_login,omitempty"`
	Disabled       int64              `json:"disabled,omitempty"`
	DisabledReason string             `json:"disabled_reason,omitempty"`
	Expires        int64              `json:"expires,omitempty"`
	DeviceCount    int                `json:"device_count"`
	MaxDevices     int                `json:"max_devices"`
	Sessions       int                `json:"sessions"`
	ResetPending   bool               `json:"reset_pending,omitempty"`
	Devices        []DeviceInfo       `json:"devices,omitempty"`
	Events         []EntitlementEvent `json:"events,omitempty"`
}

func (s *Store) userInfo(state *ledger, id string, user userRecord, now time.Time, detail bool) UserInfo {
	info := UserInfo{ID: id, Username: user.Username, Created: user.Created, LastLogin: user.LastLogin, Disabled: user.Disabled,
		DisabledReason: user.DisabledReason, Expires: user.Expires, DeviceCount: len(user.Devices), MaxDevices: s.maxDevices(),
		ResetPending: user.Reset != nil && user.Reset.Expires > now.Unix()}
	switch {
	case user.Disabled != 0:
		info.Status = UserDisabled
	case user.Expires > now.Unix():
		info.Status = UserActive
	case user.Expires > 0:
		info.Status = UserExpired
	default:
		info.Status = UserNone
	}
	for _, session := range state.Sessions {
		if session.User == id {
			info.Sessions++
		}
	}
	if detail {
		info.Devices = s.accountInfo(id, user, "", now).Devices
		info.Events = slices.Clone(user.Events)
		slices.Reverse(info.Events)
	}
	return info
}

// Users lists every account for administrators.
func (s *Store) Users(now time.Time) ([]UserInfo, error) {
	var users []UserInfo
	err := s.read(func(state *ledger) {
		users = make([]UserInfo, 0, len(state.Users))
		for id, user := range state.Users {
			users = append(users, s.userInfo(state, id, user, now, false))
		}
	})
	return users, err
}

// User returns one account with its devices and term history.
func (s *Store) User(id string, now time.Time) (UserInfo, error) {
	var info UserInfo
	found := false
	err := s.read(func(state *ledger) {
		if user, ok := state.Users[id]; ok {
			info, found = s.userInfo(state, id, user, now, true), true
		}
	})
	if err == nil && !found {
		err = ErrNotFound
	}
	return info, err
}

func (s *Store) changeUser(id string, now time.Time, change func(state *ledger, user *userRecord) (bool, error)) (UserInfo, error) {
	var info UserInfo
	err := s.update(func(state *ledger) (bool, error) {
		ensureAccounts(state)
		user, ok := state.Users[id]
		if !ok {
			return false, ErrNotFound
		}
		changed, err := change(state, &user)
		if err != nil {
			return false, err
		}
		state.Users[id] = user
		info = s.userInfo(state, id, user, now, true)
		return changed, nil
	})
	return info, err
}

// SetUserDisabled disables or restores an account. Disabling signs out every
// device; grants already issued expire within AccountGrantTerm.
func (s *Store) SetUserDisabled(id string, disabled bool, reason string, now time.Time) (UserInfo, error) {
	return s.changeUser(id, now, func(state *ledger, user *userRecord) (bool, error) {
		if disabled == (user.Disabled != 0) {
			return false, nil
		}
		if disabled {
			user.Disabled, user.DisabledReason = now.Unix(), reason
			for key, session := range state.Sessions {
				if session.User == id {
					delete(state.Sessions, key)
				}
			}
		} else {
			user.Disabled, user.DisabledReason = 0, ""
		}
		return true, nil
	})
}

// AdjustUser adds (or with negative days removes) days of term.
func (s *Store) AdjustUser(id string, days int, note string, now time.Time) (UserInfo, error) {
	if days == 0 || days > 3650 || days < -3650 {
		return UserInfo{}, errors.New("天数需在 -3650 至 3650 之间且不为 0")
	}
	return s.changeUser(id, now, func(_ *ledger, user *userRecord) (bool, error) {
		user.extend(now, int64(days)*86400, SourceAdmin, "", note)
		return true, nil
	})
}

// RemoveUserDevice frees a device slot and signs that device out.
func (s *Store) RemoveUserDevice(id, device string, now time.Time) (UserInfo, error) {
	return s.changeUser(id, now, func(state *ledger, user *userRecord) (bool, error) {
		if !removeDevice(state, id, user, device) {
			return false, ErrNotFound
		}
		return true, nil
	})
}

// IssueReset creates a one-time password reset code valid for a day. Only
// its digest is stored; support passes the code to the user.
func (s *Store) IssueReset(id string, now time.Time) (string, int64, error) {
	raw, err := randomHex(6)
	if err != nil {
		return "", 0, err
	}
	code := strings.ToUpper(raw)
	expires := now.Add(resetLifetime).Unix()
	_, err = s.changeUser(id, now, func(_ *ledger, user *userRecord) (bool, error) {
		user.Reset = &resetRecord{Digest: tokenDigest(code), Expires: expires}
		return true, nil
	})
	if err != nil {
		return "", 0, err
	}
	return code[:4] + "-" + code[4:8] + "-" + code[8:], expires, nil
}

// ShortID shortens an account ID for audit lines.
func ShortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
