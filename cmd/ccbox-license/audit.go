package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/sylvinus/agent-vm/internal/host"
)

// auditLog appends administrator actions as JSON lines next to the ledger.
// It records who acted and what changed, never codes or signed grants.
type auditLog struct {
	dir string
	mu  sync.Mutex
}

type auditEntry struct {
	Time    int64  `json:"time"`
	Action  string `json:"action"`
	Detail  string `json:"detail,omitempty"`
	Address string `json:"address,omitempty"`
}

const (
	auditFile     = "admin-audit.jsonl"
	auditReadTail = 1 << 20
)

func (l *auditLog) record(now time.Time, action, detail, address string) {
	line, _ := json.Marshal(auditEntry{Time: now.Unix(), Action: action, Detail: detail, Address: address})
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := host.EnsurePrivateDir(l.dir); err != nil {
		log.Printf("administrator audit unavailable: %v", err)
		return
	}
	file, err := os.OpenFile(filepath.Join(l.dir, auditFile), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		log.Printf("administrator audit unavailable: %v", err)
		return
	}
	defer file.Close()
	if _, err := file.Write(append(line, '\n')); err != nil {
		log.Printf("administrator audit unavailable: %v", err)
	}
}

// read returns up to limit of the newest entries, newest first.
func (l *auditLog) read(limit int) ([]auditEntry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	file, err := os.Open(filepath.Join(l.dir, auditFile))
	if errors.Is(err, os.ErrNotExist) {
		return []auditEntry{}, nil
	} else if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	offset := max(info.Size()-auditReadTail, 0)
	data, err := io.ReadAll(io.NewSectionReader(file, offset, info.Size()-offset))
	if err != nil {
		return nil, err
	}
	if offset > 0 {
		if cut := bytes.IndexByte(data, '\n'); cut >= 0 {
			data = data[cut+1:]
		}
	}
	entries := []auditEntry{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		var entry auditEntry
		if json.Unmarshal(scanner.Bytes(), &entry) == nil && entry.Action != "" {
			entries = append(entries, entry)
		}
	}
	slices.Reverse(entries)
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}
