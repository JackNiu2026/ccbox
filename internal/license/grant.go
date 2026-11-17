package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const Duration = 30 * 24 * time.Hour

type Grant struct {
	Version int    `json:"version"`
	Device  string `json:"device"`
	Expires int64  `json:"expires"`
	// Account names the online account a grant was issued for. Older
	// clients ignore it; it does not change how a grant verifies.
	Account string `json:"account,omitempty"`
}

func (g Grant) Active(now time.Time) bool { return g.Expires > now.Unix() }

func ValidDevice(device string) bool {
	_, err := hex.DecodeString(device)
	return len(device) == 64 && err == nil && device == strings.ToLower(device)
}

var encoding = base64.RawURLEncoding.Strict()

func (g Grant) sign(key ed25519.PrivateKey) string {
	payload, _ := json.Marshal(g)
	return encoding.EncodeToString(payload) + "." + encoding.EncodeToString(ed25519.Sign(key, payload))
}

func Verify(key ed25519.PublicKey, token, device string) (Grant, error) {
	invalid := errors.New("授权签名无效或不属于此设备")
	if len(key) != ed25519.PublicKeySize || len(token) > 1024 || !ValidDevice(device) {
		return Grant{}, invalid
	}
	data, signature, ok := strings.Cut(token, ".")
	payload, dataErr := encoding.DecodeString(data)
	sig, sigErr := encoding.DecodeString(signature)
	var grant Grant
	if !ok || dataErr != nil || sigErr != nil || !ed25519.Verify(key, payload, sig) ||
		json.Unmarshal(payload, &grant) != nil || grant.Version != 1 || grant.Device != device || grant.Expires <= 0 {
		return Grant{}, invalid
	}
	return grant, nil
}
