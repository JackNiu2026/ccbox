package workbench

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"testing/fstest"
)

func TestPackagedWebAssetsRejectDamageAfterBuild(t *testing.T) {
	files := fstest.MapFS{
		"index.html":     {Data: []byte(`<title>CCBox</title><link rel="icon" href="/ccbox.svg"><script type="module" src="/assets/main.js"></script>`)},
		"mobile.html":    {Data: []byte(`<script type="module" src="/assets/main.js"></script>`)},
		"assets/main.js": {Data: []byte("export const valid=true;")},
	}
	hashes := map[string]string{}
	for name, file := range files {
		hash := sha256.Sum256(file.Data)
		hashes[name] = hex.EncodeToString(hash[:])
	}
	manifest, _ := json.Marshal(map[string]any{"version": 1, "files": hashes})
	files["build-integrity.json"] = &fstest.MapFile{Data: manifest}
	if err := verifyWebAssets(files); err != nil {
		t.Fatal(err)
	}
	branded := files["index.html"].Data
	files["index.html"].Data = []byte(`<title>Cage</title><link rel="icon" href="/ccbox.svg">`)
	hash := sha256.Sum256(files["index.html"].Data)
	hashes["index.html"] = hex.EncodeToString(hash[:])
	files["build-integrity.json"].Data, _ = json.Marshal(map[string]any{"version": 1, "files": hashes})
	if err := verifyWebAssets(files); err == nil {
		t.Fatal("accepted upstream branding with a valid integrity manifest")
	}
	files["index.html"].Data = branded
	files["build-integrity.json"].Data = manifest
	files["assets/main.js"].Data = make([]byte, 20)
	if err := verifyWebAssets(files); err == nil {
		t.Fatal("damaged embedded JavaScript accepted")
	}
	delete(files, "assets/main.js")
	if err := verifyWebAssets(files); err == nil {
		t.Fatal("missing embedded JavaScript accepted")
	}
	delete(files, "build-integrity.json")
	if err := verifyWebAssets(files); err == nil {
		t.Fatal("unverified build accepted")
	}
}
