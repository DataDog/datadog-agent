// +build ebpf_bindata

package bytecode

import (
	"bytes"
)

// GetReader returns a new AssetReader for the specified bundled asset
func GetReader(dir, name string) (AssetReader, error) {
	content, err := Asset(name)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(content), nil
}
