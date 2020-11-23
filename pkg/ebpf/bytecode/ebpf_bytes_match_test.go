// +build linux_bpf,ebpf_bindata

package bytecode

import (
	"fmt"
	"io/ioutil"
	"path"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/bindata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEbpfBytesCorrect(t *testing.T) {
	dir := "build"
	for _, filename := range bindata.AssetNames() {
		bs, err := ioutil.ReadFile(path.Join(dir, filename))
		require.NoError(t, err)

		actualReader, err := GetReader(dir, filename)
		require.NoError(t, err)

		actual, err := ioutil.ReadAll(actualReader)
		require.NoError(t, err)

		assert.Equal(t, bs, actual, fmt.Sprintf("on-disk file %s and bundled content are different", filename))
	}
}
