package workbench

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sylvinus/agent-vm/internal/host"
	"github.com/sylvinus/agent-vm/internal/license"
)

// Set the public key at build time for licensed desktop distributions.
// A build without a key retains the existing preview behavior.
var licensePublicKey string

var licenseEndpoint = "https://ccbox.org/v1/redeem"

var (
	errLicenseRequired = errors.New("请在设置 → 授权中激活或续期后再启动环境")
	activationMu       sync.Mutex
	activationHTTP     = &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
)

type licenseView struct {
	State   string `json:"state"`
	Device  string `json:"device,omitempty"`
	Expires int64  `json:"expires,omitempty"`
}

func (s *Server) licenseEnabled() bool { return licensePublicKey != "" && s.localVMStorage() }

func (s *Server) licenseStatus() (licenseView, error) {
	if !s.licenseEnabled() {
		return licenseView{State: "disabled"}, nil
	}
	device, err := license.DeviceID()
	if err != nil {
		return licenseView{}, err
	}
	view := licenseView{State: "inactive", Device: device}
	path := filepath.Join(s.profile, "license.json")
	grant, err := loadLicense(path, device)
	if errors.Is(err, os.ErrNotExist) {
		return view, nil
	}
	if err != nil {
		view.State = "invalid"
		return view, nil
	}
	view.Expires = grant.Expires
	view.State = "expired"
	if grant.Active(time.Now()) {
		view.State = "active"
	}
	return view, nil
}

func loadLicense(path, device string) (license.Grant, error) {
	if err := host.CheckPrivateFile(path); err != nil {
		return license.Grant{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return license.Grant{}, err
	}
	defer f.Close()
	var token string
	if err := json.NewDecoder(io.LimitReader(f, 2048)).Decode(&token); err != nil {
		return license.Grant{}, err
	}
	return verifyLicense(token, device)
}

func verifyLicense(token, device string) (license.Grant, error) {
	key, err := base64.RawURLEncoding.Strict().DecodeString(licensePublicKey)
	if err != nil {
		return license.Grant{}, err
	}
	return license.Verify(ed25519.PublicKey(key), token, device)
}

func (s *Server) requireLicense() error {
	view, err := s.licenseStatus()
	if err != nil {
		return err
	}
	if view.State != "active" && view.State != "disabled" {
		return errLicenseRequired
	}
	return nil
}

func (s *Server) desktopLicense(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.hasTrustedView(r) || !s.licenseEnabled() {
		if r.Method == http.MethodGet {
			jsonReply(w, licenseView{State: "disabled"})
		} else {
			http.Error(w, "请在本机 CCBox 窗口激活", http.StatusForbidden)
		}
		return
	}
	if r.Method == http.MethodGet {
		view, err := s.licenseStatus()
		if err != nil {
			requestError(w, err)
			return
		}
		jsonReply(w, view)
		return
	}
	var input struct {
		Code string `json:"code"`
	}
	if !readJSON(w, r, &input, 512) {
		return
	}
	if len(input.Code) == 0 || len(input.Code) > 64 {
		requestError(w, license.ErrInvalid)
		return
	}
	device, err := license.DeviceID()
	if err != nil {
		requestError(w, err)
		return
	}
	activationMu.Lock()
	defer activationMu.Unlock()
	path := filepath.Join(s.profile, "license.json")
	token, err := redeemLicense(r.Context(), input.Code, device)
	if err != nil {
		requestError(w, err)
		return
	}
	grant, err := verifyLicense(token, device)
	if err != nil || !grant.Active(time.Now()) {
		requestError(w, errors.New("授权签名无效或该激活码对应的授权已到期"))
		return
	}
	old, err := loadLicense(path, device)
	if err != nil || old.Expires < grant.Expires {
		if err := writePrivate(path, token); err != nil {
			requestError(w, errors.New("无法保存授权，请使用同一激活码重试"))
			return
		}
	} else {
		grant = old
	}
	jsonReply(w, licenseView{State: "active", Device: device, Expires: grant.Expires})
}

func redeemLicense(ctx context.Context, code, device string) (string, error) {
	body, _ := json.Marshal(map[string]string{"code": code, "device": device})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, licenseEndpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	unavailable := errors.New("无法连接 CCBox 授权服务，请检查网络后重试")
	response, err := activationHTTP.Do(req)
	if err != nil {
		return "", unavailable
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusBadRequest:
		return "", license.ErrInvalid
	case http.StatusConflict:
		return "", license.ErrUsed
	case http.StatusOK:
		var reply struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(io.LimitReader(response.Body, 2048)).Decode(&reply); err == nil {
			return reply.Token, nil
		}
	}
	return "", unavailable
}
