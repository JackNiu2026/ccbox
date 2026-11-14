package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sylvinus/agent-vm/internal/host"
	"github.com/sylvinus/agent-vm/internal/license"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	if len(args) == 2 && args[0] == "init-key" {
		public, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		if err = saveKey(args[1], private.Seed()); err != nil {
			return err
		}
		fmt.Println(base64.RawURLEncoding.EncodeToString(public))
		return nil
	}
	if len(args) < 3 {
		return errors.New("usage: ccbox-license init-key KEY | issue STATE KEY | serve STATE KEY | transfer STATE KEY FROM TO")
	}
	if err := host.CheckPrivateFile(args[2]); err != nil {
		return err
	}
	data, err := os.ReadFile(args[2])
	if err != nil {
		return err
	}
	seed, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return errors.New("invalid license signing key")
	}
	store := license.NewStore(args[1], ed25519.NewKeyFromSeed(seed))
	clear(seed)
	switch {
	case args[0] == "issue" && len(args) == 3:
		code, err := store.Issue()
		if err == nil {
			fmt.Println(code)
		}
		return err
	case args[0] == "transfer" && len(args) == 5:
		token, err := store.Transfer(args[3], args[4], time.Now())
		if err == nil {
			fmt.Println(token)
		}
		return err
	case args[0] == "serve" && len(args) == 3:
		mux := http.NewServeMux()
		mux.HandleFunc("POST /v1/redeem", redeemHandler(store))
		mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok\n") })
		if path := os.Getenv("CCBOX_LICENSE_ADMIN_PASSWORD_FILE"); path != "" {
			if err := host.CheckPrivateFile(path); err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			password := strings.TrimSpace(string(data))
			if len(password) < 24 {
				return errors.New("administrator password must have at least 24 characters")
			}
			mux.HandleFunc("GET /admin/license/{$}", adminHandler(store, password))
			mux.HandleFunc("POST /admin/license/{$}", adminHandler(store, password))
		}
		address := "127.0.0.1:8791"
		if os.Getenv("CCBOX_LICENSE_CONTAINER") == "1" {
			address = ":8791"
		}
		server := &http.Server{Addr: address, Handler: mux,
			ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
			WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8 << 10}
		return server.ListenAndServe()
	default:
		return errors.New("invalid license command")
	}
}

func saveKey(path string, seed []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if err = host.ProtectFile(path); err == nil {
		_, err = fmt.Fprintln(file, base64.RawURLEncoding.EncodeToString(seed))
	}
	if err == nil {
		err = file.Sync()
	}
	if err = errors.Join(err, file.Close()); err != nil {
		_ = os.Remove(path)
	}
	return err
}

func redeemHandler(store *license.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Code   string `json:"code"`
			Device string `json:"device"`
		}
		w.Header().Set("Cache-Control", "no-store")
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF ||
			len(input.Code) > 64 || !license.ValidDevice(input.Device) {
			http.Error(w, license.ErrInvalid.Error(), http.StatusBadRequest)
			return
		}
		token, err := store.Redeem(input.Code, input.Device, time.Now())
		switch {
		case errors.Is(err, license.ErrInvalid):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case errors.Is(err, license.ErrUsed):
			http.Error(w, err.Error(), http.StatusConflict)
		case err != nil:
			log.Printf("license ledger unavailable: %v", err)
			http.Error(w, "授权服务暂时不可用", http.StatusServiceUnavailable)
		default:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"token": token})
		}
	}
}
