package license

import (
	"crypto/ed25519"
	"errors"
	"strings"
	"testing"
	"time"
)

func accountStore(t *testing.T) (*Store, ed25519.PublicKey) {
	t.Helper()
	pub, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return NewStore(t.TempDir(), private), pub
}

var (
	deviceA = strings.Repeat("a", 64)
	deviceB = strings.Repeat("b", 64)
	deviceC = strings.Repeat("c", 64)
	t0      = time.Unix(1770000000, 0)
)

func TestRegisterLoginAndNames(t *testing.T) {
	store, _ := accountStore(t)
	token, info, err := store.Register("Alice", "password1", deviceA, DeviceMeta{Name: "Office PC\n", Platform: "windows"}, t0)
	if err != nil || len(token) != 64 || info.Username != "Alice" || info.Active || len(info.Devices) != 1 || !info.Devices[0].Current || info.Devices[0].Name != "Office PC" {
		t.Fatalf("register: %q %+v %v", token, info, err)
	}
	if _, _, err := store.Register("alice", "password2", deviceB, DeviceMeta{}, t0); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("case-insensitive duplicate allowed: %v", err)
	}
	for _, bad := range []string{"ab", "has space", strings.Repeat("x", 33), "semi;colon"} {
		if _, _, err := store.Register(bad, "password1", deviceB, DeviceMeta{}, t0); err == nil {
			t.Fatalf("invalid name %q accepted", bad)
		}
	}
	if _, _, err := store.Register("bob", "short", deviceB, DeviceMeta{}, t0); err == nil {
		t.Fatal("short password accepted")
	}
	if _, _, err := store.Login("ALICE", "wrong-pass", deviceB, DeviceMeta{}, t0); !errors.Is(err, ErrCredentials) {
		t.Fatalf("wrong password: %v", err)
	}
	if _, _, err := store.Login("nobody", "password1", deviceB, DeviceMeta{}, t0); !errors.Is(err, ErrCredentials) {
		t.Fatalf("unknown user: %v", err)
	}
	second, info, err := store.Login("ALICE", "password1", deviceB, DeviceMeta{}, t0)
	if err != nil || second == token || len(info.Devices) != 2 {
		t.Fatalf("login: %+v %v", info, err)
	}
	if _, err := store.Account(second, deviceA, t0); !errors.Is(err, ErrSession) {
		t.Fatalf("session used from another device: %v", err)
	}
	if _, _, err := store.Login("alice", "password1", deviceC, DeviceMeta{}, t0); !errors.Is(err, ErrDeviceLimit) {
		t.Fatalf("device limit: %v", err)
	}
	// Signing in again on a known device does not use another slot.
	if _, _, err := store.Login("alice", "password1", deviceA, DeviceMeta{}, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RemoveDevice(second, deviceB, deviceA, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Account(token, deviceA, t0); !errors.Is(err, ErrSession) {
		t.Fatalf("removed device kept its session: %v", err)
	}
	if _, _, err := store.Login("alice", "password1", deviceC, DeviceMeta{}, t0); err != nil {
		t.Fatalf("freed slot not reusable: %v", err)
	}
}

func TestRedeemIntoAccountAndGrant(t *testing.T) {
	store, pub := accountStore(t)
	token, _, err := store.Register("carol", "password1", deviceA, DeviceMeta{}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AccountGrant(token, deviceA, t0); !errors.Is(err, ErrNoEntitlement) {
		t.Fatalf("grant without term: %v", err)
	}
	code, _ := store.Issue()
	info, err := store.RedeemToAccount(token, deviceA, code, t0)
	if err != nil || !info.Active || info.Expires != t0.Add(Duration).Unix() {
		t.Fatalf("redeem: %+v %v", info, err)
	}
	if _, err := store.RedeemToAccount(token, deviceA, code, t0); !errors.Is(err, ErrUsed) {
		t.Fatalf("code reused: %v", err)
	}
	if _, err := store.Redeem(code, deviceB, t0); !errors.Is(err, ErrUsed) {
		t.Fatalf("account code redeemed again by a device: %v", err)
	}
	second, _ := store.Issue()
	info, _ = store.RedeemToAccount(token, deviceA, second, t0.Add(time.Hour))
	if info.Expires != t0.Add(2*Duration).Unix() {
		t.Fatalf("second code did not stack: %d", info.Expires)
	}
	signed, _, err := store.AccountGrant(token, deviceA, t0)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := Verify(pub, signed, deviceA)
	if err != nil || grant.Expires != t0.Add(AccountGrantTerm).Unix() || grant.Account != info.ID {
		t.Fatalf("grant: %+v %v", grant, err)
	}
	snapshot, _ := store.Snapshot()
	for _, item := range snapshot.Codes {
		if item.Status != StatusRedeemed || item.Account != info.ID {
			t.Fatalf("code record: %+v", item)
		}
	}
}

func TestLegacyTermMovesIntoAccountOnce(t *testing.T) {
	store, _ := accountStore(t)
	code, _ := store.Issue()
	if _, err := store.Redeem(code, deviceA, t0); err != nil {
		t.Fatal(err)
	}
	_, info, err := store.Register("dave", "password1", deviceA, DeviceMeta{}, t0.Add(time.Hour))
	if err != nil || info.Expires != t0.Add(Duration).Unix() {
		t.Fatalf("legacy term not imported: %+v %v", info, err)
	}
	_, other, err := store.Register("erin", "password1", deviceA, DeviceMeta{}, t0.Add(time.Hour))
	if err != nil || other.Expires != 0 {
		t.Fatalf("legacy term imported twice: %+v %v", other, err)
	}
	// An older client on the linked device still extends the account.
	renewal, _ := store.Issue()
	if _, err := store.Redeem(renewal, deviceA, t0.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	users, _ := store.Users(t0)
	for _, user := range users {
		if user.Username == "dave" && user.Expires != t0.Add(2*Duration).Unix() {
			t.Fatalf("legacy redeem did not extend the owner: %+v", user)
		}
	}
}

func TestAdministrationAndPasswords(t *testing.T) {
	store, _ := accountStore(t)
	token, info, _ := store.Register("frank", "password1", deviceA, DeviceMeta{}, t0)
	other, _, _ := store.Login("frank", "password1", deviceB, DeviceMeta{}, t0)
	user, err := store.AdjustUser(info.ID, 10, "补偿", t0)
	if err != nil || user.Status != UserActive || user.Expires != t0.Add(240*time.Hour).Unix() || user.Events[0].Note != "补偿" {
		t.Fatalf("adjust: %+v %v", user, err)
	}
	if user, _ = store.AdjustUser(info.ID, -20, "", t0); user.Expires != t0.Unix() || user.Status != UserExpired {
		t.Fatalf("negative adjustment: %+v", user)
	}
	if _, err := store.AdjustUser(info.ID, 0, "", t0); err == nil {
		t.Fatal("zero adjustment accepted")
	}
	if user, _ = store.SetUserDisabled(info.ID, true, "abuse", t0); user.Status != UserDisabled || user.Sessions != 0 {
		t.Fatalf("disable: %+v", user)
	}
	if _, _, err := store.Login("frank", "password1", deviceA, DeviceMeta{}, t0); !errors.Is(err, ErrAccountOff) {
		t.Fatalf("disabled login: %v", err)
	}
	if _, err := store.Account(token, deviceA, t0); !errors.Is(err, ErrSession) {
		t.Fatalf("disabled session kept: %v", err)
	}
	store.SetUserDisabled(info.ID, false, "", t0)
	token, _, _ = store.Login("frank", "password1", deviceA, DeviceMeta{}, t0)
	other, _, _ = store.Login("frank", "password1", deviceB, DeviceMeta{}, t0)
	if err := store.ChangePassword(token, deviceA, "wrong-pass", "password2", t0); !errors.Is(err, ErrCredentials) {
		t.Fatalf("change with wrong password: %v", err)
	}
	if err := store.ChangePassword(token, deviceA, "password1", "password2", t0); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Account(token, deviceA, t0); err != nil {
		t.Fatalf("current session lost: %v", err)
	}
	if _, err := store.Account(other, deviceB, t0); !errors.Is(err, ErrSession) {
		t.Fatalf("other session kept after password change: %v", err)
	}
	code, expires, err := store.IssueReset(info.ID, t0)
	if err != nil || len(code) != 14 || expires != t0.Add(24*time.Hour).Unix() {
		t.Fatalf("reset code: %q %v", code, err)
	}
	if err := store.ResetPassword("frank", "0000-0000-0000", "password3", t0); !errors.Is(err, ErrResetInvalid) {
		t.Fatalf("wrong reset code: %v", err)
	}
	if err := store.ResetPassword("FRANK", strings.ToLower(code), "password3", t0); err != nil {
		t.Fatal(err)
	}
	if err := store.ResetPassword("frank", code, "password4", t0); !errors.Is(err, ErrResetInvalid) {
		t.Fatalf("reset code reused: %v", err)
	}
	if _, _, err := store.Login("frank", "password3", deviceA, DeviceMeta{}, t0); err != nil {
		t.Fatalf("login after reset: %v", err)
	}
	user, _ = store.RemoveUserDevice(info.ID, deviceB, t0)
	if user.DeviceCount != 1 || len(user.Devices) != 1 {
		t.Fatalf("remove device: %+v", user)
	}
	expired, _, _ := store.Login("frank", "password3", deviceB, DeviceMeta{}, t0)
	if _, err := store.Account(expired, deviceB, t0.Add(sessionLifetime+time.Hour)); !errors.Is(err, ErrSession) {
		t.Fatalf("expired session accepted: %v", err)
	}
}
