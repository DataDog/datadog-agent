// +build !ebpf_bindata

package bytecode

import (
	"os"
	"path"

	"github.com/pkg/errors"
)

// GetReader returns a new AssetReader for the specified file asset
func GetReader(name string) (AssetReader, error) {
	asset, err := os.Open(path.Join(DefaultBPFDir, path.Base(name)))
	if err != nil {
		return nil, errors.Wrap(err, "could not find asset")
	}

	return asset, nil
}
