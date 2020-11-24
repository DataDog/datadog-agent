package bytecode

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// RuntimeAsset represents an asset that needs it content integrity checked at runtime
type RuntimeAsset struct {
	filename string
	hash     string
}

func NewRuntimeAsset(filename, hash string) *RuntimeAsset {
	return &RuntimeAsset{
		filename: filename,
		hash:     hash,
	}
}

// Verify reads the asset in the provided directory and verifies the content hash matches what is expected.
// On success, it returns the full path and content hash of the asset.
func (a *RuntimeAsset) Verify(dir string) (string, string, error) {
	p := filepath.Join(dir, a.filename)
	f, err := os.Open(p)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", "", fmt.Errorf("error hashing file %s: %w", f.Name(), err)
	}
	if fmt.Sprintf("%x", h.Sum(nil)) != a.hash {
		return "", "", fmt.Errorf("file content hash does not match expected value")
	}
	return p, a.hash, nil
}
