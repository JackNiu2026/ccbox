package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sylvinus/agent-vm/internal/host"
)

var (
	ErrInvalid = errors.New("激活码无效")
	ErrUsed    = errors.New("激活码已被其他设备使用")
)

const MaxIssueBatch = 500

type Store struct {
	dir string
	key ed25519.PrivateKey
}

type codeRecord struct {
	Device string `json:"device,omitempty"`
	Token  string `json:"token,omitempty"`
}

type ledger struct {
	Codes   map[string]codeRecord `json:"codes"`
	Expires map[string]int64      `json:"expires"`
}

func NewStore(dir string, key ed25519.PrivateKey) *Store { return &Store{dir: dir, key: key} }

func (s *Store) update(change func(*ledger) (bool, error)) error {
	if len(s.key) != ed25519.PrivateKeySize {
		return errors.New("invalid license signing key")
	}
	if err := host.EnsurePrivateDir(s.dir); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(s.dir, "ledger.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := lockLedger(lock); err != nil {
		return err
	}
	path := filepath.Join(s.dir, "ledger.json")
	current := ledger{Codes: map[string]codeRecord{}, Expires: map[string]int64{}}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &current); err != nil {
			return err
		}
		if current.Codes == nil || current.Expires == nil {
			return errors.New("invalid activation ledger")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	changed, err := change(&current)
	if err != nil || !changed {
		return err
	}
	return writeLedger(s.dir, path, current)
}

func writeLedger(dir, path string, value ledger) error {
	file, err := os.CreateTemp(dir, ".ledger-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = host.ProtectFile(file.Name()); err == nil {
		err = json.NewEncoder(file).Encode(value)
	}
	if err == nil {
		err = file.Sync()
	}
	if err = errors.Join(err, file.Close()); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return err
	}
	return syncLedgerDirectory(dir)
}

func codeDigest(code string) string {
	normalized := strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(code)), "-", "")
	raw, err := hex.DecodeString(normalized)
	if len(normalized) != 32 || err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s *Store) Issue() (string, error) {
	return s.issue(nil)
}

func (s *Store) issue(prepare func(*ledger) (codeRecord, error)) (string, error) {
	codes, err := s.issueBatch(1, prepare)
	if err != nil {
		return "", err
	}
	return codes[0], nil
}

func (s *Store) IssueBatch(count int) ([]string, error) {
	return s.issueBatch(count, nil)
}

func (s *Store) issueBatch(count int, prepare func(*ledger) (codeRecord, error)) ([]string, error) {
	if count < 1 || count > MaxIssueBatch {
		return nil, errors.New("invalid activation code quantity")
	}
	codes := make([]string, 0, count)
	err := s.update(func(state *ledger) (bool, error) {
		var record codeRecord
		if prepare != nil {
			var err error
			record, err = prepare(state)
			if err != nil {
				return false, err
			}
		}
		for len(codes) < count {
			var raw [16]byte
			if _, err := rand.Read(raw[:]); err != nil {
				return false, err
			}
			hexCode := strings.ToUpper(hex.EncodeToString(raw[:]))
			code := strings.Join([]string{hexCode[:8], hexCode[8:16], hexCode[16:24], hexCode[24:]}, "-")
			digest := codeDigest(code)
			if _, exists := state.Codes[digest]; exists {
				continue
			}
			state.Codes[digest] = record
			codes = append(codes, code)
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return codes, nil
}

func (s *Store) Redeem(code, device string, now time.Time) (string, error) {
	digest := codeDigest(code)
	if digest == "" || !ValidDevice(device) {
		return "", ErrInvalid
	}
	var token string
	err := s.update(func(state *ledger) (bool, error) {
		record, ok := state.Codes[digest]
		if !ok {
			return false, ErrInvalid
		}
		if record.Device != "" {
			if record.Device != device {
				return false, ErrUsed
			}
			token = record.Token
			return false, nil
		}
		expires := max(now.Unix(), state.Expires[device]) + int64(Duration/time.Second)
		token = Grant{Version: 1, Device: device, Expires: expires}.sign(s.key)
		state.Codes[digest] = codeRecord{Device: device, Token: token}
		state.Expires[device] = expires
		return true, nil
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

// Transfer issues the remaining term to another device. Existing signed
// offline grants remain valid until their original expiry.
func (s *Store) Transfer(from, to string, now time.Time) (string, error) {
	if !ValidDevice(from) || !ValidDevice(to) || from == to {
		return "", errors.New("invalid source or destination device")
	}
	return s.issue(func(state *ledger) (codeRecord, error) {
		expires := state.Expires[from]
		if expires <= now.Unix() || state.Expires[to] > now.Unix() {
			return codeRecord{}, errors.New("transfer requires an active source and an inactive destination")
		}
		delete(state.Expires, from)
		state.Expires[to] = expires
		return codeRecord{Device: to, Token: Grant{Version: 1, Device: to, Expires: expires}.sign(s.key)}, nil
	})
}
