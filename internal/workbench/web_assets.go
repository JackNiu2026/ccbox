package workbench

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
)

// VerifyWebAssets checks the bytes embedded in this executable against the
// frontend builder's manifest, before a build is accepted for packaging.
func VerifyWebAssets() error {
	web, err := fs.Sub(assets, "web")
	if err != nil {
		return err
	}
	return verifyWebAssets(web)
}

func verifyWebAssets(web fs.FS) error {
	data, err := fs.ReadFile(web, "build-integrity.json")
	if err != nil {
		return fmt.Errorf("前端完整性清单缺失，请重新构建界面：%w", err)
	}
	var manifest struct {
		Version int               `json:"version"`
		Files   map[string]string `json:"files"`
	}
	if json.Unmarshal(data, &manifest) != nil || manifest.Version != 1 || manifest.Files["index.html"] == "" || manifest.Files["mobile.html"] == "" {
		return fmt.Errorf("前端完整性清单无效，请重新构建界面")
	}
	for name, want := range manifest.Files {
		data, err := fs.ReadFile(web, name)
		if err != nil {
			return fmt.Errorf("前端资源缺失：%s", name)
		}
		hash := sha256.Sum256(data)
		if hex.EncodeToString(hash[:]) != want {
			return fmt.Errorf("前端资源损坏：%s，请重新构建界面", name)
		}
		if name == "index.html" && (!bytes.Contains(data, []byte("<title>CCBox</title>")) ||
			!bytes.Contains(data, []byte(`href="/ccbox.svg"`))) {
			return fmt.Errorf("内嵌工作台的品牌与 CCBox 不一致，请使用当前仓库重新构建")
		}
	}
	return nil
}
