package license

import (
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRedeemAndRenew(t *testing.T) {
	pub, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir()
	store := NewStore(path, private)
	now := time.Unix(1770000000, 0)
	device := strings.Repeat("a", 64)
	other := strings.Repeat("b", 64)
	code, err := store.Issue()
	if err != nil {
		t.Fatal(err)
	}
	token, err := store.Redeem(code, device, now)
	if err != nil {
		t.Fatal(err)
	}
	first, err := Verify(pub, token, device)
	if err != nil || first.Expires != now.Add(Duration).Unix() {
		t.Fatalf("first activation: %+v %v", first, err)
	}
	if _, err := Verify(pub, token, other); err == nil {
		t.Fatal("copied grant accepted")
	}
	if _, err := Verify(pub, token+"a", device); err == nil {
		t.Fatal("tampered grant accepted")
	}
	store = NewStore(path, private)
	retry, err := store.Redeem(strings.ToLower(code), device, now.Add(time.Hour))
	if err != nil || retry != token {
		t.Fatalf("retry changed the grant: %v", err)
	}
	if _, err := store.Redeem(code, other, now); !errors.Is(err, ErrUsed) {
		t.Fatalf("another device redeemed the code: %v", err)
	}
	for _, tc := range []struct{ when, expires time.Time }{
		{now.Add(time.Hour), now.Add(2 * Duration)},
		{now.Add(3 * Duration), now.Add(4 * Duration)},
	} {
		code, err := store.Issue()
		if err != nil {
			t.Fatal(err)
		}
		token, err := store.Redeem(code, device, tc.when)
		if err != nil {
			t.Fatal(err)
		}
		grant, err := Verify(pub, token, device)
		if err != nil || grant.Expires != tc.expires.Unix() || grant.Active(tc.expires) {
			t.Fatalf("renewal: %+v %v", grant, err)
		}
	}
}

func TestConcurrentRedemption(t *testing.T) {
	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(t.TempDir(), private)
	code, err := store.Issue()
	if err != nil {
		t.Fatal(err)
	}
	start, results := make(chan struct{}), make(chan error, 2)
	var ready sync.WaitGroup
	for _, character := range []string{"a", "b"} {
		ready.Go(func() {
			<-start
			_, err := store.Redeem(code, strings.Repeat(character, 64), time.Now())
			results <- err
		})
	}
	close(start)
	ready.Wait()
	a, b := <-results, <-results
	if !((a == nil && errors.Is(b, ErrUsed)) || (b == nil && errors.Is(a, ErrUsed))) {
		t.Fatalf("expected one redemption: %v, %v", a, b)
	}
}

func TestTransferKeepsRemainingTerm(t *testing.T) {
	pub, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(t.TempDir(), private)
	from := strings.Repeat("a", 64)
	to := strings.Repeat("b", 64)
	now := time.Unix(1770000000, 0)
	code, err := store.Issue()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Redeem(code, from, now); err != nil {
		t.Fatal(err)
	}
	transfer, err := store.Transfer(from, to, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Redeem(transfer, from, now); !errors.Is(err, ErrUsed) {
		t.Fatalf("transfer can be redeemed elsewhere: %v", err)
	}
	token, err := store.Redeem(transfer, to, now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	grant, err := Verify(pub, token, to)
	if err != nil || grant.Expires != now.Add(Duration).Unix() {
		t.Fatalf("wrong transferred term: %+v %v", grant, err)
	}
}

func TestAdministrationMetadata(t *testing.T) {
	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(t.TempDir(), private)
	now := time.Unix(1770000000, 0)
	device := strings.Repeat("c", 64)
	batch, codes, err := store.IssueLabeled(3, " 渠道 A ", now)
	if err != nil || len(codes) != 3 || batch == "" {
		t.Fatalf("labeled batch: %q %v %v", batch, codes, err)
	}
	if _, _, err := store.IssueLabeled(1, strings.Repeat("长", MaxNoteLength+1), now); err == nil {
		t.Fatal("overlong note accepted")
	}
	if _, err := store.Redeem(codes[0], device, now); err != nil {
		t.Fatal(err)
	}
	if err := store.Revoke(CodeID(codes[0]), now); err == nil {
		t.Fatal("redeemed code revoked")
	}
	if err := store.Revoke(CodeID(codes[1]), now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Redeem(codes[1], device, now); !errors.Is(err, ErrRevoked) {
		t.Fatalf("revoked code redeemed: %v", err)
	}
	if changed, err := store.RevokeBatch(batch, now); err != nil || changed != 1 {
		t.Fatalf("batch revoke changed %d: %v", changed, err)
	}
	renewal, expires, err := store.Renew(device, now)
	if err != nil || expires != now.Add(2*Duration).Unix() {
		t.Fatalf("renewal: %d %v", expires, err)
	}
	if _, err := store.Redeem(renewal, strings.Repeat("d", 64), now); !errors.Is(err, ErrUsed) {
		t.Fatalf("renewal redeemed elsewhere: %v", err)
	}
	if _, err := store.Redeem(renewal, device, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot()
	if err != nil || len(snapshot.Codes) != 4 || len(snapshot.Batches) != 1 || snapshot.Batches[0].Note != "渠道 A" {
		t.Fatalf("snapshot: %+v %v", snapshot, err)
	}
	statuses := map[string]int{}
	for _, code := range snapshot.Codes {
		statuses[code.Kind+"/"+code.Status]++
		if code.Hint == "" || code.Issued == 0 {
			t.Fatalf("missing metadata: %+v", code)
		}
	}
	if statuses["code/redeemed"] != 1 || statuses["code/revoked"] != 2 || statuses["renewal/redeemed"] != 1 {
		t.Fatalf("statuses: %v", statuses)
	}
	if snapshot.Expires[device] != expires {
		t.Fatal("device expiry not reported")
	}
}

func TestLegacyLedgerStillReads(t *testing.T) {
	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	device := strings.Repeat("e", 64)
	token := Grant{Version: 1, Device: device, Expires: 1770000000}.sign(private)
	legacy := `{"codes":{"` + strings.Repeat("1", 64) + `":{},"` + strings.Repeat("2", 64) + `":{"device":"` + device + `","token":"` + token + `"}},"expires":{"` + device + `":1770000000}}`
	if err := os.WriteFile(filepath.Join(dir, "ledger.json"), []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := NewStore(dir, private).Snapshot()
	if err != nil || len(snapshot.Codes) != 2 {
		t.Fatalf("legacy snapshot: %+v %v", snapshot, err)
	}
	for _, code := range snapshot.Codes {
		if code.Device != "" && (code.Status != StatusRedeemed || code.Expires != 1770000000) {
			t.Fatalf("legacy redeemed code: %+v", code)
		}
		if code.Device == "" && code.Status != StatusUnused {
			t.Fatalf("legacy unused code: %+v", code)
		}
	}
}
