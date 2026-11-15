package host

import (
	"errors"
	"strings"
	"testing"
)

func TestWHvHypervisorPresent(t *testing.T) {
	const insufficientBuffer = 0x80370301 // WHV_E_INSUFFICIENT_BUFFER
	for _, tc := range []struct {
		name    string
		status  uintptr
		value   uint64
		wantErr string
	}{
		{"running", 0, 1, ""},
		{"running, S_FALSE", 1, 1, ""},
		{"BOOL is the low four bytes", 0, 0xffffffff_00000001, ""},
		{"not running", 0, 0, errHypervisorNotRunning.Error()},
		{"not running, high bytes set", 0, 0xffffffff_00000000, errHypervisorNotRunning.Error()},
		{"query rejected", insufficientBuffer, 0, "HRESULT 0x80370301"},
		{"sign-extended HRESULT", uintptr(0xffffffff_80370301), 1, "HRESULT 0x80370301"},
	} {
		err := whvHypervisorPresent(tc.status, tc.value)
		if tc.wantErr == "" {
			if err != nil {
				t.Fatalf("%s: unexpected error %v", tc.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Fatalf("%s: got %v, want %q", tc.name, err, tc.wantErr)
		}
	}
	// A failed query must not tell a working host to change firmware settings.
	if err := whvHypervisorPresent(insufficientBuffer, 0); errors.Is(err, errHypervisorNotRunning) || strings.Contains(err.Error(), "固件") {
		t.Fatalf("query failure reported as disabled virtualization: %v", err)
	}
}
