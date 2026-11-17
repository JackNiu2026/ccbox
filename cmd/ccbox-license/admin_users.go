package main

import (
	"cmp"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sylvinus/agent-vm/internal/license"
)

// --- Users ------------------------------------------------------------------

func (a *adminServer) registerUsers(mux *http.ServeMux) {
	mux.HandleFunc("GET "+adminPrefix+"api/users", a.api(true, a.listUsers))
	mux.HandleFunc("GET "+adminPrefix+"api/users/detail", a.api(true, a.userDetail))
	mux.HandleFunc("POST "+adminPrefix+"api/users/disable", a.api(true, a.disableUser))
	mux.HandleFunc("POST "+adminPrefix+"api/users/enable", a.api(true, a.enableUser))
	mux.HandleFunc("POST "+adminPrefix+"api/users/adjust", a.api(true, a.adjustUser))
	mux.HandleFunc("POST "+adminPrefix+"api/users/devices/remove", a.api(true, a.removeUserDevice))
	mux.HandleFunc("POST "+adminPrefix+"api/users/reset", a.api(true, a.resetUser))
}

func validUserID(id string) bool {
	return len(id) == 32 && validHex(id)
}

func validHex(value string) bool {
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

const maxReason = 80

func cleanReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if utf8.RuneCountInString(reason) > maxReason {
		return "", badRequest("原因最多 " + strconv.Itoa(maxReason) + " 个字符")
	}
	return reason, nil
}

func userTotals(users []license.UserInfo, now time.Time) map[string]int {
	totals := map[string]int{"total": len(users)}
	for _, user := range users {
		totals[user.Status]++
		if now.Unix()-user.Created <= 30*86400 {
			totals["new30"]++
		}
		if user.Status == license.UserActive && user.Expires-now.Unix() <= 7*86400 {
			totals["expiring"]++
		}
	}
	return totals
}

func (a *adminServer) listUsers(r *http.Request, _ string) (any, error) {
	users, err := a.store.Users(a.now())
	if err != nil {
		return nil, err
	}
	query := r.URL.Query()
	status := query.Get("status")
	search := strings.ToLower(strings.TrimSpace(query.Get("q")))
	rows := slices.DeleteFunc(slices.Clone(users), func(user license.UserInfo) bool {
		if status != "" && user.Status != status {
			return true
		}
		return search != "" && !strings.Contains(strings.ToLower(user.Username), search) && !strings.HasPrefix(user.ID, search)
	})
	slices.SortFunc(rows, func(x, y license.UserInfo) int {
		if x.Created != y.Created {
			return cmp.Compare(y.Created, x.Created)
		}
		return strings.Compare(x.ID, y.ID)
	})
	page, size := pageParams(r)
	result := paginate(rows, page, size)
	result["totals"] = userTotals(users, a.now())
	return result, nil
}

func (a *adminServer) userDetail(r *http.Request, _ string) (any, error) {
	id := r.URL.Query().Get("id")
	if !validUserID(id) {
		return nil, badRequest("用户编号无效")
	}
	user, err := a.store.User(id, a.now())
	if err != nil {
		return nil, err
	}
	entries, err := a.audit.read(500)
	if err != nil {
		return nil, err
	}
	short := license.ShortID(id)
	related := slices.DeleteFunc(entries, func(entry auditEntry) bool {
		return !strings.HasPrefix(entry.Action, "users.") || !strings.HasPrefix(entry.Detail, short)
	})
	if len(related) > 50 {
		related = related[:50]
	}
	return map[string]any{"user": user, "audit": related}, nil
}

type userAction struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
	Days   int    `json:"days"`
	Device string `json:"device"`
}

func (a *adminServer) userInput(r *http.Request) (userAction, error) {
	var input userAction
	if err := decodeBody(r, &input); err != nil {
		return input, err
	}
	if !validUserID(input.ID) {
		return input, badRequest("用户编号无效")
	}
	reason, err := cleanReason(input.Reason)
	input.Reason = reason
	return input, err
}

func auditDetail(id string, parts ...string) string {
	detail := license.ShortID(id)
	for _, part := range parts {
		if part != "" {
			detail += " · " + part
		}
	}
	return detail
}

func (a *adminServer) disableUser(r *http.Request, _ string) (any, error) {
	input, err := a.userInput(r)
	if err != nil {
		return nil, err
	}
	if input.Reason == "" {
		return nil, badRequest("请填写停用原因")
	}
	now := a.now()
	user, err := a.store.SetUserDisabled(input.ID, true, input.Reason, now)
	if err != nil {
		return nil, err
	}
	a.audit.record(now, "users.disable", auditDetail(input.ID, user.Username, input.Reason), clientAddress(r))
	return user, nil
}

func (a *adminServer) enableUser(r *http.Request, _ string) (any, error) {
	input, err := a.userInput(r)
	if err != nil {
		return nil, err
	}
	now := a.now()
	user, err := a.store.SetUserDisabled(input.ID, false, "", now)
	if err != nil {
		return nil, err
	}
	a.audit.record(now, "users.enable", auditDetail(input.ID, user.Username, input.Reason), clientAddress(r))
	return user, nil
}

func (a *adminServer) adjustUser(r *http.Request, _ string) (any, error) {
	input, err := a.userInput(r)
	if err != nil {
		return nil, err
	}
	if input.Days == 0 || input.Days > 3650 || input.Days < -3650 {
		return nil, badRequest("天数需在 -3650 至 3650 之间且不为 0")
	}
	if input.Reason == "" {
		return nil, badRequest("请填写调整原因")
	}
	now := a.now()
	user, err := a.store.AdjustUser(input.ID, input.Days, input.Reason, now)
	if err != nil {
		return nil, err
	}
	days := strconv.Itoa(input.Days)
	if input.Days > 0 {
		days = "+" + days
	}
	a.audit.record(now, "users.adjust", auditDetail(input.ID, user.Username, days+" 天", input.Reason), clientAddress(r))
	return user, nil
}

func (a *adminServer) removeUserDevice(r *http.Request, _ string) (any, error) {
	input, err := a.userInput(r)
	if err != nil {
		return nil, err
	}
	input.Device = strings.ToLower(strings.TrimSpace(input.Device))
	if !license.ValidDevice(input.Device) {
		return nil, badRequest("设备编号应为 64 位十六进制字符")
	}
	now := a.now()
	user, err := a.store.RemoveUserDevice(input.ID, input.Device, now)
	if err != nil {
		return nil, err
	}
	a.audit.record(now, "users.device-remove", auditDetail(input.ID, user.Username, input.Device[:12], input.Reason), clientAddress(r))
	return user, nil
}

func (a *adminServer) resetUser(r *http.Request, _ string) (any, error) {
	input, err := a.userInput(r)
	if err != nil {
		return nil, err
	}
	now := a.now()
	code, expires, err := a.store.IssueReset(input.ID, now)
	if errors.Is(err, license.ErrNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	user, err := a.store.User(input.ID, now)
	if err != nil {
		return nil, err
	}
	a.audit.record(now, "users.reset", auditDetail(input.ID, user.Username, input.Reason), clientAddress(r))
	return map[string]any{"code": code, "expires": expires, "user": user}, nil
}
