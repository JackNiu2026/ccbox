package cage

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAbsoluteGuestOpenerStaysInNumberedDesktop(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh unavailable")
	}
	root := filepath.ToSlash(t.TempDir())
	runtime := root + "/run/ccbox-claude-desktop-2"
	if err := os.MkdirAll(runtime, 0700); err != nil {
		t.Fatal(err)
	}
	script := strings.NewReplacer(
		"/run/ccbox-claude-desktop-", "$test_root/run/ccbox-claude-desktop-",
		"/usr/local/bin/ccbox-instance-open", "printf '%s'",
	).Replace(guestOpen)
	script = `test_root=$(cd "$2" && pwd -P) || exit
export HOME="$test_root/.local/share/ccbox-desktop-instances/2/home"
export CCBOX_DESKTOP_DATA="${HOME%/home}/app" XDG_RUNTIME_DIR="$test_root/run/ccbox-claude-desktop-2$3"
unset CAGE_CLAUDE_LOGIN
sudo() { return 11; }
set -- "$1"
` + script
	for _, tc := range []struct {
		url, runtimeSuffix string
		ok                 bool
	}{
		{"https://example.invalid/login", "", true},
		{"claude://callback", "", true},
		{"claude://callback", "-unrelated", false},
	} {
		cmd := exec.Command(shell, "-c", script, "open", tc.url, root, tc.runtimeSuffix)
		out, err := cmd.CombinedOutput()
		if tc.ok && (err != nil || string(out) != tc.url) || !tc.ok && err == nil {
			t.Fatalf("opener routing: %q, %v; wanted success=%v", out, err, tc.ok)
		}
	}
}

func TestDesktopCallbackFocusAfterHandoff(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	for _, identity := range []string{"cage", "cage-browser"} {
		for _, tc := range []struct {
			name, url, path, login string
			fail, parameter        bool
		}{
			{"callback", "claude://login?code=private-test-value", "/desktop-focus", "", false, false},
			{"failed-callback", "claude://login?code=private-test-value", "", "", true, false},
			{"web-link", "https://example.com/login", "/new", "", false, false},
			{"cli-login", "https://claude.ai/oauth/authorize?state=private-test-value", "/login-open", "1234567890abcdef", false, false},
			{"finish-cli-login", "--finish-login", "/login-finish", "1234567890abcdef", false, false},
			{"cli-login-argument", "https://claude.ai/oauth/authorize?state=private-test-value", "/login-open", strings.Repeat("a", 24), false, true},
			{"finish-cli-login-argument", "--finish-login", "/login-finish", strings.Repeat("a", 24), false, true},
		} {
			t.Run(identity+"/"+tc.name, func(t *testing.T) {
				dir := t.TempDir()
				type request struct{ Path, Auth, URL, Login string }
				requests := make(chan request, 2)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var body struct {
						URL   string `json:"url"`
						Login string `json:"login"`
					}
					_ = json.NewDecoder(r.Body).Decode(&body)
					requests <- request{r.URL.Path, r.Header.Get("Authorization"), body.URL, body.Login}
					w.Write([]byte(`{"ok":true}`))
				}))
				defer server.Close()
				config := filepath.Join(dir, "control.json")
				if err := os.WriteFile(config, []byte(`{"token":"test-only"}`), 0600); err != nil {
					t.Fatal(err)
				}
				// Preserve the production shell flow; sudo only substitutes the two
				// guest identities and the app. Node performs the real HTTP request.
				fixture := `id() { printf '%s\n' "$TEST_IDENTITY"; }
sudo() {
	shift
	user=$1
	shift
	if [ "$user" = cage ]; then return "$CALLBACK_EXIT"; fi
	if [ "$TEST_IDENTITY" = cage-browser ]; then echo 'sudo denied' >&2; return 1; fi
	shift
	"$TEST_NODE" "$@"
}
`
				configJSON, _ := json.Marshal(filepath.ToSlash(config))
				script := strings.ReplaceAll(guestOpen, `"/etc/claude-cage/browser-control.json"`, string(configJSON))
				script = strings.ReplaceAll(script, "http://127.0.0.1:18500", server.URL)
				script = strings.ReplaceAll(script, "/usr/bin/node", "\"$TEST_NODE\"")
				code := "0"
				if tc.fail {
					code = "1"
				}
				args := []string{"-c", fixture + script, "open"}
				login := tc.login
				if tc.parameter {
					args = append(args, "--login-id", tc.login)
					login = ""
				}
				args = append(args, tc.url)
				cmd := exec.Command("sh", args...)
				cmd.Env = append(os.Environ(), "MSYS2_ARG_CONV_EXCL=*", "TEST_NODE="+filepath.ToSlash(node), "TEST_IDENTITY="+identity, "CALLBACK_EXIT="+code, "CAGE_CLAUDE_LOGIN="+login)
				out, err := cmd.CombinedOutput()
				if tc.fail {
					if err == nil {
						t.Fatal("failed handoff reported success")
					}
					select {
					case r := <-requests:
						t.Fatalf("failed handoff emitted focus: %+v", r)
					default:
					}
					return
				}
				if err != nil {
					t.Fatalf("%v: %s", err, out)
				}
				select {
				case r := <-requests:
					wantURL := ""
					if tc.path == "/new" || tc.path == "/login-open" {
						wantURL = tc.url
					}
					if r.Path != tc.path || r.Auth != "Bearer test-only" || r.URL != wantURL || r.Login != tc.login {
						t.Fatalf("unexpected request: %+v", r)
					}
				default:
					t.Fatal("handoff notification missing")
				}
			})
		}
	}
}

func TestTailnetBrowserOpenPassesOnlyReturnSignal(t *testing.T) {
	bin := t.TempDir()
	helper := filepath.Join(bin, "return-helper")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\""), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "sudo"), []byte("#!/bin/sh\nshift 4\nexec \"$@\""), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "id"), []byte("#!/bin/sh\nprintf '%s' \"$TEST_IDENTITY\""), 0755); err != nil {
		t.Fatal(err)
	}
	script := strings.ReplaceAll(guestOpen, "/usr/local/bin/cage-tailnet", helper)
	for _, identity := range []string{"cage", "cage-browser"} {
		cmd := exec.Command("sh", "-c", script, "open", "tailscale://return?code=private-fixture")
		cmd.Env = append(os.Environ(), "PATH="+bin+":/usr/bin:/bin", "TEST_IDENTITY="+identity)
		if out, err := cmd.CombinedOutput(); err != nil || string(out) != "--browser-return\n" {
			t.Fatalf("unexpected handoff for %s: %q %v", identity, out, err)
		}
	}
}

func TestClaudeDesktopInstallerRejectsUntrustedKey(t *testing.T) {
	dir := t.TempDir()
	commands := map[string]string{
		"sudo":      "#!/bin/sh\nif [ \"$1\" = env ]; then exec \"$@\"; fi\necho blocked-privileged-operation >&2; exit 97\n",
		"apt-get":   "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$HOME/apt-requests\"\n",
		"systemctl": "#!/bin/sh\nexit 3\n",
		"flock":     "#!/bin/sh\nexit 0\n",
		"dpkg":      "#!/bin/sh\necho arm64\n",
		"curl":      "#!/bin/sh\nexit 0\n",
		"gpg":       "#!/bin/sh\necho 'fpr:::::::::UNTRUSTEDKEY:'\n",
	}
	for name, body := range commands {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0755); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("bash", "-s")
	cmd.Stdin = strings.NewReader(ClaudeDesktopInstallScript())
	cmd.Env = append(os.Environ(), "HOME="+dir, "PATH="+dir+":"+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "signing key did not match") {
		t.Fatalf("untrusted key was not rejected: %v %s", err, out)
	}
	requests, _ := os.ReadFile(filepath.Join(dir, "apt-requests"))
	if strings.Contains(string(requests), "claude-desktop") {
		t.Fatal("desktop package requested before source authentication")
	}
}

func TestClaudeDesktopScriptsParse(t *testing.T) {
	for name, script := range map[string]string{"install": ClaudeDesktopInstallScript(), "provision": ClaudeDesktopProvisionScript(), "session": claudeDesktopSession, "start": claudeDesktopStart, "open": claudeDesktopOpen, "browser-open": guestOpen} {
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command("bash", "-n")
			cmd.Stdin = strings.NewReader(script)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v: %s", err, out)
			}
		})
	}
}
