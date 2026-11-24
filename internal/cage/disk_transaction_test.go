package cage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDiskResizeBacksUpAndRestoresExistingData(t *testing.T) {
	dir := t.TempDir()
	disk := filepath.Join(dir, "diffdisk")
	yaml := filepath.Join(dir, "lima.yaml")
	write := func(path, contents string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(disk, "original disk")
	write(yaml, "disk: 24GiB\n")
	if err := BeginDiskResize(context.Background(), dir, "current-job"); err != nil {
		t.Fatal(err)
	}
	if err := AllowDiskResizeStart(dir, "another-job"); err == nil {
		t.Fatal("an interrupted resize must block an unrelated startup")
	}
	if err := AllowDiskResizeStart(dir, "current-job"); err != nil {
		t.Fatal(err)
	}
	if err := BeginDiskResize(context.Background(), dir, "another-job"); err == nil {
		t.Fatal("a new job must not replace the only recovery backups")
	}
	write(disk, "partial resize")
	write(yaml, "disk: 16GiB\n")
	if err := RestoreDiskResize(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	gotDisk, _ := os.ReadFile(disk)
	gotYAML, _ := os.ReadFile(yaml)
	if string(gotDisk) != "original disk" || string(gotYAML) != "disk: 24GiB\n" {
		t.Fatalf("restore did not recover the original disk and config: %q, %q", gotDisk, gotYAML)
	}
	if err := AllowDiskResizeStart(dir, "another-job"); err != nil {
		t.Fatal(err)
	}
}
