// +build linux_bpf

package ebpf

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/compiler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompilerMatch(t *testing.T) {
	cPath := "../network/ebpf/c/prebuilt/offset-guess.c"
	if _, err := os.Stat(cPath); err != nil {
		if os.IsNotExist(err) {
			t.Skipf("compiler test must be run in source tree")
		} else {
			t.Fatalf("error checking for offset-guess.c: %s", err)
		}
		return
	}

	cfg := NewDefaultConfig()

	c, err := compiler.NewEBPFCompiler(nil, false)
	require.NoError(t, err)
	defer c.Close()

	cflags := []string{
		"-I./c",
		"-I../network/ebpf/c",
		"-includeasm_goto_workaround.h",
	}
	tmpObjFile, err := ioutil.TempFile("", "offset-guess-static-*.o")
	require.NoError(t, err)
	defer os.Remove(tmpObjFile.Name())

	onDiskObjFilename := tmpObjFile.Name()
	err = c.CompileToObjectFile(cPath, onDiskObjFilename, cflags)
	require.NoError(t, err)

	bs, err := ioutil.ReadFile(onDiskObjFilename)
	require.NoError(t, err)

	bundleFilename := "offset-guess.o"
	actualReader, err := bytecode.GetReader(cfg.BPFDir, bundleFilename)
	require.NoError(t, err)
	defer actualReader.Close()

	actual, err := ioutil.ReadAll(actualReader)
	require.NoError(t, err)

	assert.Equal(t, bs, actual, fmt.Sprintf("prebuilt file %s and statically-linked clang compiled content %s are different", bundleFilename, onDiskObjFilename))
}
