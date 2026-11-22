package workbench

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestVaultEncryptionLockReopenPasswordChangeAndCorruption(t *testing.T) {
	a := newTestAccount(t)
	ctx := context.Background()
	err := a.withVault(ctx, true, func(db *vaultData) error {
		return db.save(vaultEntry{Origin: "https://mail.example.com/login", Username: "private@example.com", Password: "secret-mail-password"})
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filepath.Dir(a.path), "password-vault.json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, plain := range []string{"private@example.com", "secret-mail-password", "mail.example.com"} {
		if bytes.Contains(original, []byte(plain)) {
			t.Fatal("plaintext persisted")
		}
	}
	if err = a.changePassword("initial-test-password", "changed-account-password"); err != nil {
		t.Fatal(err)
	}
	oldLease := a.vaultLease
	a.lock()
	if err = a.withVault(ctx, false, func(*vaultData) error { t.Fatal("locked callback executed"); return nil }); err == nil {
		t.Fatal("locked read accepted")
	}
	reopened, err := loadLocalAccount(filepath.Dir(a.path))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.lock()
	if err = reopened.unlock("changed-account-password"); err != nil {
		t.Fatal(err)
	}
	if reopened.vaultLease == oldLease {
		t.Fatal("unlock reused browser lease")
	}
	err = reopened.withVault(ctx, false, func(db *vaultData) error {
		if len(db.Entries) != 1 || db.Entries[0].Password != "secret-mail-password" || db.Entries[0].Origin != "https://mail.example.com" {
			t.Fatalf("data lost after password change/reopen: count %d", len(db.Entries))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var envelope vaultEnvelope
	if json.Unmarshal(original, &envelope) != nil {
		t.Fatal("invalid envelope")
	}
	envelope.Data[0] ^= 1
	corrupted, _ := json.Marshal(envelope)
	if err = os.WriteFile(path, corrupted, 0600); err != nil {
		t.Fatal(err)
	}
	if err = reopened.withVault(ctx, true, func(db *vaultData) error { return db.save(vaultEntry{Origin: "https://new.example", Password: "new"}) }); err == nil {
		t.Fatal("corrupt vault overwritten")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(corrupted, after) {
		t.Fatal("corrupt vault changed")
	}
}

func TestVaultOriginAndDuplicateUpdate(t *testing.T) {
	db := vaultData{}
	for _, raw := range []string{"http://mail.example.com", "https://user:pass@mail.example.com", "file:///test", "https://"} {
		if db.save(vaultEntry{Origin: raw, Password: "secret"}) == nil {
			t.Fatal("invalid origin accepted", raw)
		}
	}
	if err := db.save(vaultEntry{Origin: "https://MAIL.example.com:443/login", Username: "alice", Password: "old"}); err != nil {
		t.Fatal(err)
	}
	if db.save(vaultEntry{Origin: "https://mail.example.com", Username: "alice", Password: "duplicate"}) == nil {
		t.Fatal("duplicate accepted")
	}
	if err := db.save(vaultEntry{ID: db.Entries[0].ID, Origin: "https://mail.example.com", Username: "alice"}); err != nil {
		t.Fatal(err)
	}
	if len(db.Entries) != 1 || db.Entries[0].Password != "old" {
		t.Fatal("blank edit lost existing password")
	}
	if err := db.save(vaultEntry{Origin: "https://mail.example.com", Username: "bob", Password: "other"}); err != nil {
		t.Fatal(err)
	}
}
