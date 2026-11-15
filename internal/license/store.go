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
	ErrInvalid  = errors.New("激活码无效")
	ErrUsed     = errors.New("激活码已被其他设备使用")
	ErrRevoked  = errors.New("激活码已停用")
	ErrNotFound = errors.New("未找到对应记录")
)

const MaxIssueBatch = 500

type Store struct {
	dir string
	key ed25519.PrivateKey
}

// Record kinds. Ordinary codes may be redeemed by any device; transfer and
// renewal codes are bound to one device when they are issued.
const (
	KindCode     = "code"
	KindTransfer = "transfer"
	KindRenewal  = "renewal"
)

type codeRecord struct {
	Device   string `json:"device,omitempty"`
	Token    string `json:"token,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Issued   int64  `json:"issued,omitempty"`
	Batch    string `json:"batch,omitempty"`
	Hint     string `json:"hint,omitempty"`
	Redeemed int64  `json:"redeemed,omitempty"`
	Revoked  int64  `json:"revoked,omitempty"`
	From     string `json:"from,omitempty"`
	Expires  int64  `json:"expires,omitempty"`
}

type batchRecord struct {
	Created int64  `json:"created"`
	Count   int    `json:"count"`
	Note    string `json:"note,omitempty"`
}

type ledger struct {
	Codes   map[string]codeRecord  `json:"codes"`
	Expires map[string]int64       `json:"expires"`
	Batches map[string]batchRecord `json:"batches,omitempty"`
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

// IssueLabeled issues a named batch so administrators can find its codes
// later without the service keeping the codes themselves.
func (s *Store) IssueLabeled(count int, note string, now time.Time) (string, []string, error) {
	note = strings.TrimSpace(note)
	if len([]rune(note)) > MaxNoteLength {
		return "", nil, errors.New("备注过长")
	}
	var id string
	codes, err := s.issueBatch(count, func(state *ledger) (codeRecord, error) {
		var suffix [3]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return codeRecord{}, err
		}
		id = now.UTC().Format("20060102") + "-" + strings.ToUpper(hex.EncodeToString(suffix[:]))
		if state.Batches == nil {
			state.Batches = map[string]batchRecord{}
		}
		state.Batches[id] = batchRecord{Created: now.Unix(), Count: count, Note: note}
		return codeRecord{Kind: KindCode, Issued: now.Unix(), Batch: id}, nil
	})
	if err != nil {
		return "", nil, err
	}
	return id, codes, nil
}

// MaxNoteLength bounds batch notes, measured in characters.
const MaxNoteLength = 80

func (s *Store) issueBatch(count int, prepare func(*ledger) (codeRecord, error)) ([]string, error) {
	if count < 1 || count > MaxIssueBatch {
		return nil, errors.New("invalid activation code quantity")
	}
	codes := make([]string, 0, count)
	err := s.update(func(state *ledger) (bool, error) {
		codes = codes[:0]
		record := codeRecord{Kind: KindCode, Issued: time.Now().Unix()}
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
			record.Hint = hexCode[28:]
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
		if record.Revoked != 0 {
			return false, ErrRevoked
		}
		if record.Device != "" {
			if record.Device != device {
				return false, ErrUsed
			}
			token = record.Token
			return s.redeemBound(state, digest, now), nil
		}
		expires := max(now.Unix(), state.Expires[device]) + int64(Duration/time.Second)
		token = Grant{Version: 1, Device: device, Expires: expires}.sign(s.key)
		record.Device, record.Token, record.Redeemed, record.Expires = device, token, now.Unix(), expires
		state.Codes[digest] = record
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
		return codeRecord{Kind: KindTransfer, Issued: now.Unix(), From: from, Expires: expires,
			Device: to, Token: Grant{Version: 1, Device: to, Expires: expires}.sign(s.key)}, nil
	})
}

// Renew extends one device by a full term and returns a code that only that
// device can redeem. It serves support cases such as compensation.
func (s *Store) Renew(device string, now time.Time) (string, int64, error) {
	if !ValidDevice(device) {
		return "", 0, errors.New("设备编号无效")
	}
	var expires int64
	code, err := s.issue(func(state *ledger) (codeRecord, error) {
		expires = max(now.Unix(), state.Expires[device]) + int64(Duration/time.Second)
		state.Expires[device] = expires
		return codeRecord{Kind: KindRenewal, Issued: now.Unix(), Expires: expires,
			Device: device, Token: Grant{Version: 1, Device: device, Expires: expires}.sign(s.key)}, nil
	})
	return code, expires, err
}

// Revoke disables an unused, unbound code. Redeemed codes cannot be revoked:
// their signed grants already live offline on the device.
func (s *Store) Revoke(id string, now time.Time) error {
	return s.update(func(state *ledger) (bool, error) {
		record, ok := state.Codes[id]
		switch {
		case !ok:
			return false, ErrNotFound
		case record.Revoked != 0:
			return false, nil
		case record.Device != "":
			return false, errors.New("已兑换或绑定设备的激活码不能停用")
		}
		record.Revoked = now.Unix()
		state.Codes[id] = record
		return true, nil
	})
}

// RevokeBatch disables every unused code of a batch and reports how many
// codes changed.
func (s *Store) RevokeBatch(batch string, now time.Time) (int, error) {
	changed := 0
	err := s.update(func(state *ledger) (bool, error) {
		if _, ok := state.Batches[batch]; !ok {
			return false, ErrNotFound
		}
		for id, record := range state.Codes {
			if record.Batch == batch && record.Device == "" && record.Revoked == 0 {
				record.Revoked = now.Unix()
				state.Codes[id] = record
				changed++
			}
		}
		return changed > 0, nil
	})
	return changed, err
}

// CodeID returns the ledger identifier of a code, or "" when malformed.
func CodeID(code string) string { return codeDigest(code) }

// CodeInfo is the administrator view of a code; it never includes the code
// or the signed grant.
type CodeInfo struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Status   string `json:"status"`
	Hint     string `json:"hint,omitempty"`
	Batch    string `json:"batch,omitempty"`
	Issued   int64  `json:"issued,omitempty"`
	Redeemed int64  `json:"redeemed,omitempty"`
	Revoked  int64  `json:"revoked,omitempty"`
	Device   string `json:"device,omitempty"`
	From     string `json:"from,omitempty"`
	Expires  int64  `json:"expires,omitempty"`
}

type BatchInfo struct {
	ID      string `json:"id"`
	Created int64  `json:"created"`
	Count   int    `json:"count"`
	Note    string `json:"note,omitempty"`
}

type Snapshot struct {
	Codes   []CodeInfo
	Expires map[string]int64
	Batches []BatchInfo
}

// Code statuses shown to administrators.
const (
	StatusUnused   = "unused"
	StatusRedeemed = "redeemed"
	StatusPending  = "pending"
	StatusRevoked  = "revoked"
)

func (record codeRecord) info(id string) CodeInfo {
	info := CodeInfo{ID: id, Kind: record.Kind, Hint: record.Hint, Batch: record.Batch, Issued: record.Issued,
		Redeemed: record.Redeemed, Revoked: record.Revoked, Device: record.Device, From: record.From, Expires: record.Expires}
	if info.Kind == "" {
		info.Kind = KindCode
	}
	switch {
	case record.Revoked != 0:
		info.Status = StatusRevoked
	case record.Device == "":
		info.Status = StatusUnused
	case record.Redeemed != 0 || (info.Kind == KindCode && record.Token != ""):
		info.Status = StatusRedeemed
	default:
		info.Status = StatusPending
	}
	if info.Expires == 0 && record.Token != "" {
		if data, _, ok := strings.Cut(record.Token, "."); ok {
			if payload, err := encoding.DecodeString(data); err == nil {
				var grant Grant
				if json.Unmarshal(payload, &grant) == nil {
					info.Expires = grant.Expires
				}
			}
		}
	}
	return info
}

// Snapshot reads the ledger for administration without changing it.
func (s *Store) Snapshot() (Snapshot, error) {
	var snapshot Snapshot
	err := s.update(func(state *ledger) (bool, error) {
		snapshot.Codes = make([]CodeInfo, 0, len(state.Codes))
		for id, record := range state.Codes {
			snapshot.Codes = append(snapshot.Codes, record.info(id))
		}
		snapshot.Expires = make(map[string]int64, len(state.Expires))
		for device, expires := range state.Expires {
			snapshot.Expires[device] = expires
		}
		for id, batch := range state.Batches {
			snapshot.Batches = append(snapshot.Batches, BatchInfo{ID: id, Created: batch.Created, Count: batch.Count, Note: batch.Note})
		}
		return false, nil
	})
	return snapshot, err
}

// redeemBound records the first redemption of a device-bound code.
func (s *Store) redeemBound(state *ledger, digest string, now time.Time) bool {
	record := state.Codes[digest]
	if record.Redeemed != 0 || record.Kind == "" || record.Kind == KindCode {
		return false
	}
	record.Redeemed = now.Unix()
	state.Codes[digest] = record
	return true
}
