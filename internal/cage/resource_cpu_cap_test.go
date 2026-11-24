package cage

import "testing"

func TestVMCPUCapacityWindows(t *testing.T) {
	for _, test := range []struct {
		platform string
		share    int
		want     int
	}{
		{"windows", 96, 16},
		{"windows", 4, 4},
		{"linux", 96, 96},
		{"darwin", 32, 32},
	} {
		if got := vmCPULimit(test.platform, test.share); got != test.want {
			t.Errorf("CPU capacity on %s: got %d, want %d", test.platform, got, test.want)
		}
	}
}
