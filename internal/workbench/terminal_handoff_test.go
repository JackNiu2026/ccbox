package workbench

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/sylvinus/agent-vm/internal/guestos"
)

func TestTerminalHandoffChoiceIsSavedOnlyForNewConversations(t *testing.T) {
	s := testServer(t)
	workspace := t.TempDir()
	s.macosGuest = &guestos.MacOSProfile{Instance: "mac", User: "cage"}
	if err := writePrivate(filepath.Join(s.profile, "environment.json"), map[string]string{"directory": workspace}); err != nil {
		t.Fatal(err)
	}
	s.preferences.Workspace = workspace
	for _, test := range []struct {
		name   string
		choice *bool
		policy string
	}{
		{"inherit", nil, "ask"},
		{"enable", new(true), "allow"},
		{"disable", new(false), "wait"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, created, err := s.prepareTerminalConversation(workspace, claudeSelection{}, "", test.choice)
			if err != nil {
				t.Fatal(err)
			}
			if created.SwitchPolicy != test.policy {
				t.Fatalf("new policy %q, want %q", created.SwitchPolicy, test.policy)
			}
			_, resumed, err := s.prepareTerminalConversation(workspace, claudeSelection{}, created.ID, new(test.policy != "allow"))
			if err != nil {
				t.Fatal(err)
			}
			if resumed.ID != created.ID || resumed.SwitchPolicy != test.policy {
				t.Fatalf("resumed policy %q, want preserved %q", resumed.SwitchPolicy, test.policy)
			}
		})
	}
}

func TestClaudeTerminalManualAccountSelectionLeavesAutomaticPoolUntouched(t *testing.T) {
	s := testServer(t)
	account := claudeAccount{ID: strings.Repeat("a", 24), ProfileID: strings.Repeat("b", 24),
		Source: "managed", Label: "Manual", Status: "ready", Email: "manual@example.test"}
	if err := s.saveClaudeAccounts(claudeAccountList{Accounts: []claudeAccount{account}}); err != nil {
		t.Fatal(err)
	}
	selected, err := s.selectTerminalClaudeAccount("claude", account.ID, false, s.workspace())
	want := claudeSelection{ID: account.ID, GuestID: account.ProfileID, Subject: claudeIdentityHash(account.Email, account.Organization)}
	if err != nil || selected != want {
		t.Fatalf("manual selection %+v, want %+v, error %v", selected, want, err)
	}
	list, err := s.loadClaudeAccounts()
	if err != nil || list.find(account.ID) == nil || list.find(account.ID).Enabled {
		t.Fatalf("selecting an account manually altered pool eligibility: %v", err)
	}
	if _, err := s.selectTerminalClaudeAccount("claude --custom", account.ID, false, s.workspace()); err == nil {
		t.Fatal("custom launcher accepted a managed account override")
	}
}
