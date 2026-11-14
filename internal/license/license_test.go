package license

import (
	"crypto/ed25519"
	"errors"
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
