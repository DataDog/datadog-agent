// +build ebpf_bindata

package bytecode

import (
	"bytes"

	bindata "github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/bindata"
)

// GetReader returns a new AssetReader for the specified bundled asset
func GetReader(dir, name string) (AssetReader, error) {
	content, err := bindata.Asset(name)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(content), nil
}
