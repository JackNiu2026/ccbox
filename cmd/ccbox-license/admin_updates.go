package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sylvinus/agent-vm/internal/appupdate/catalog"
	"github.com/sylvinus/agent-vm/internal/host"
	"golang.org/x/mod/semver"
)

const desktopUpdateFeedPath = "/downloads/updates.json"
const desktopCatalogURL = catalog.SiteOrigin + "/downloads/catalog.json"
const maxUpdateMetadata = 64 << 10

type updatePublication struct {
	Revision uint64           `json:"revision"`
	Enabled  bool             `json:"enabled"`
	Release  *catalog.Release `json:"release,omitempty"`
	Changed  int64            `json:"changed,omitempty"`
}

type updateChannel struct {
	dir    string
	client *http.Client
	mu     sync.Mutex
}

func newUpdateChannel(dir string) *updateChannel {
	return &updateChannel{dir: dir, client: &http.Client{
		Timeout: 6 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

func (c *updateChannel) path() string { return filepath.Join(c.dir, "desktop-update.json") }

func (c *updateChannel) read() (state updatePublication, err error) {
	file, err := os.Open(c.path())
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, maxUpdateMetadata))
	if err = decoder.Decode(&state); err != nil {
		return state, err
	}
	if decoder.Decode(new(any)) != io.EOF || (state.Enabled && state.Release == nil) {
		return state, errors.New("invalid desktop update state")
	}
	return state, nil
}

func (c *updateChannel) save(state updatePublication) error {
	if err := host.EnsurePrivateDir(c.dir); err != nil {
		return err
	}
	file, err := os.CreateTemp(c.dir, ".desktop-update-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = host.ProtectFile(file.Name()); err == nil {
		err = json.NewEncoder(file).Encode(state)
	}
	if err == nil {
		err = file.Sync()
	}
	if err = errors.Join(err, file.Close()); err != nil {
		return err
	}
	return os.Rename(file.Name(), c.path())
}

func (c *updateChannel) register(mux *http.ServeMux) {
	mux.HandleFunc("GET "+desktopUpdateFeedPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		state, err := c.read()
		if err != nil {
			writeError(w, err)
			return
		}
		releases := []catalog.Release{}
		if state.Enabled {
			releases = append(releases, *state.Release)
		}
		writeJSON(w, http.StatusOK, releases)
	})
}

func (c *updateChannel) request(ctx context.Context, method, address string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, address, nil)
	if err != nil {
		return nil, err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, errors.New("release file is not available")
	}
	return response, nil
}

func (c *updateChannel) readMetadata(ctx context.Context, address string) ([]byte, error) {
	response, err := c.request(ctx, http.MethodGet, address)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxUpdateMetadata+1))
	if err == nil && len(data) > maxUpdateMetadata {
		err = errors.New("release metadata exceeds size limit")
	}
	return data, err
}

func (c *updateChannel) candidate(ctx context.Context) (*catalog.Release, error) {
	data, err := c.readMetadata(ctx, desktopCatalogURL)
	if err != nil {
		return nil, err
	}
	var entries []catalog.Release
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	entry := entries[0]
	if len(entries) != 1 || entry.Draft || entry.Prerelease || !semver.IsValid(entry.Tag) || semver.Canonical(entry.Tag) != entry.Tag || semver.Prerelease(entry.Tag) != "" {
		return nil, errors.New("invalid desktop release catalog")
	}
	result := &catalog.Release{Tag: entry.Tag}
	var checksums catalog.Asset
	for _, platform := range []struct{ os, arch string }{{"windows", "amd64"}, {"darwin", "arm64"}, {"linux", "amd64"}} {
		selected, err := catalog.Select(entries, "0.0.0", platform.os, platform.arch)
		if err != nil {
			return nil, err
		}
		if selected == nil {
			continue
		}
		if selected.Checksums.URL == "" || selected.Archive.Size > 512<<20 {
			return nil, errors.New("invalid desktop release package or checksums")
		}
		result.Assets = append(result.Assets, selected.Archive)
		checksums = selected.Checksums
	}
	if len(result.Assets) == 0 {
		return nil, nil
	}
	result.Assets = append(result.Assets, checksums)
	for _, accepted := range result.Assets {
		count := 0
		for _, asset := range entry.Assets {
			if asset.Name == accepted.Name {
				count++
			}
		}
		if count != 1 {
			return nil, errors.New("duplicate release asset")
		}
	}
	return result, nil
}

func (c *updateChannel) verify(ctx context.Context, release *catalog.Release) error {
	checksums := release.Assets[len(release.Assets)-1]
	sums, err := c.readMetadata(ctx, checksums.URL)
	if err != nil || int64(len(sums)) != checksums.Size {
		return errors.New("desktop release checksums not available")
	}
	for i := range release.Assets[:len(release.Assets)-1] {
		asset := &release.Assets[i]
		candidate := catalog.Candidate{Archive: *asset, Checksums: checksums}
		digest, err := candidate.SHA256(sums)
		if err != nil {
			return err
		}
		response, err := c.request(ctx, http.MethodHead, asset.URL)
		if err != nil {
			return err
		}
		response.Body.Close()
		if response.ContentLength != asset.Size {
			return errors.New("desktop package size differs from catalog")
		}
		asset.Digest = "sha256:" + digest
	}
	return nil
}

func (a *adminServer) viewDesktopUpdates(r *http.Request, _ string) (any, error) {
	state, err := a.updates.read()
	if err != nil {
		return nil, err
	}
	candidate, err := a.updates.candidate(r.Context())
	result := map[string]any{"publication": state, "candidate": candidate}
	if err != nil {
		result["catalog_error"] = "暂时无法读取官网发行清单，仍可暂停当前推送。"
	}
	return result, nil
}

func (a *adminServer) changeDesktopUpdates(r *http.Request, _ string) (any, error) {
	var input struct {
		Enabled  *bool  `json:"enabled"`
		Revision uint64 `json:"revision"`
		Version  string `json:"version"`
	}
	if err := decodeBody(r, &input); err != nil {
		return nil, err
	}
	tag := "v" + input.Version
	if input.Enabled == nil || !semver.IsValid(tag) || semver.Canonical(tag) != tag || semver.Prerelease(tag) != "" {
		return nil, badRequest("请选择有效的正式版本")
	}
	var candidate *catalog.Release
	if *input.Enabled {
		ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
		defer cancel()
		var err error
		candidate, err = a.updates.candidate(ctx)
		if err != nil {
			return nil, apiError{http.StatusBadGateway, "暂时无法读取官网发行清单，请稍后重试"}
		}
		if candidate == nil || candidate.Tag != tag {
			return nil, apiError{http.StatusConflict, "官网发行版本已变化，请刷新后重试"}
		}
		if err := a.updates.verify(ctx, candidate); err != nil {
			return nil, apiError{http.StatusBadGateway, "官网安装包或 SHA-256 校验清单未通过校验"}
		}
	}
	a.updates.mu.Lock()
	defer a.updates.mu.Unlock()
	state, err := a.updates.read()
	if err != nil {
		return nil, err
	}
	if state.Revision != input.Revision || (!*input.Enabled && (state.Release == nil || state.Release.Tag != tag || !state.Enabled)) {
		return nil, apiError{http.StatusConflict, "推送状态已变化，请刷新后重试"}
	}
	if candidate != nil && state.Release != nil && semver.Compare(candidate.Tag, state.Release.Tag) < 0 {
		return nil, apiError{http.StatusConflict, "推送版本不能低于上次推送的版本"}
	}
	action := "updates.pause"
	if candidate != nil {
		state.Release = candidate
		action = "updates.publish"
	}
	state.Enabled, state.Revision, state.Changed = *input.Enabled, state.Revision+1, a.now().Unix()
	if err := a.updates.save(state); err != nil {
		return nil, err
	}
	a.audit.record(a.now(), action, strings.TrimPrefix(tag, "v"), clientAddress(r))
	return state, nil
}
