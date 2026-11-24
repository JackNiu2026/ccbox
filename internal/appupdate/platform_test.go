package appupdate

import (
	"testing"

	"github.com/sylvinus/agent-vm/internal/appupdate/catalog"
)

func TestPlatformReleaseNeverInstallsAnotherPlatform(t *testing.T) {
	r := fixtureRelease("0.2.0", false)
	windowsAsset := r.Assets[0]
	windowsAsset.Name = "CCBox-0.2.0-windows-amd64-setup.exe"
	windowsAsset.URL = catalog.DownloadBase("0.2.0") + windowsAsset.Name
	androidAsset := r.Assets[0]
	androidAsset.Name = "CCBox-0.2.0-android-arm64.apk"
	androidAsset.URL = catalog.DownloadBase("0.2.0") + androidAsset.Name
	intelAsset := r.Assets[0]
	intelAsset.Name = "CCBox-0.2.0-macos-amd64.dmg"
	intelAsset.URL = catalog.DownloadBase("0.2.0") + intelAsset.Name
	r.Assets = append(r.Assets, windowsAsset, androidAsset, intelAsset)
	for _, tc := range []struct{ os, arch, name string }{
		{"windows", "amd64", windowsAsset.Name},
		{"android", "arm64", androidAsset.Name},
		{"darwin", "arm64", r.Assets[0].Name},
		{"darwin", "amd64", intelAsset.Name},
	} {
		c, err := catalog.Select([]release{r}, "0.1.0", tc.os, tc.arch)
		if err != nil || c == nil || c.Archive.Name != tc.name {
			t.Fatalf("%s/%s: %#v %v", tc.os, tc.arch, c, err)
		}
	}
	if c, err := catalog.Select([]release{fixtureRelease("0.2.0", false)}, "0.1.0", "darwin", "amd64"); err != nil || c != nil {
		t.Fatalf("Apple Silicon package selected on Intel Mac: %#v %v", c, err)
	}
	if c, err := catalog.Select([]release{fixtureRelease("0.2.0", false)}, "0.1.0", "windows", "amd64"); err != nil || c != nil {
		t.Fatalf("macOS package selected on Windows: %#v %v", c, err)
	}
	linuxAsset := windowsAsset
	linuxAsset.Name = "CCBox-0.2.0-linux-amd64.deb"
	linuxAsset.URL = catalog.DownloadBase("0.2.0") + linuxAsset.Name
	r.Assets = append(r.Assets, linuxAsset)
	if c, err := catalog.Select([]release{r}, "0.1.0", "linux", "amd64"); err != nil || c == nil || c.Archive.Name != linuxAsset.Name {
		t.Fatalf("Linux must learn about its newer package: %#v %v", c, err)
	}
	if _, err := catalog.Select([]release{r}, "0.1.0", "windows", "arm64"); err == nil {
		t.Fatal("unsupported architecture appeared up to date")
	}
}
