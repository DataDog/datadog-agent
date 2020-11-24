// +build linux_bpf

package compiler

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func curDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(file)
}

func TestCompilerMatch(t *testing.T) {
	cfg := ebpf.NewDefaultConfig()

	c, err := NewEBPFCompiler(nil, false)
	require.NoError(t, err)
	defer c.Close()

	cflags := []string{
		"-I" + filepath.Join(curDir(), "../c"),
		"-I" + filepath.Join(curDir(), "../../network/ebpf/c"),
		"-includeasm_goto_workaround.h",
	}
	tmpFile, err := ioutil.TempFile("", "offset-guess-static-*.o")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	onDiskFilename := tmpFile.Name()
	err = c.CompileToObjectFile(filepath.Join(curDir(), "../../network/ebpf/c/prebuilt/offset-guess.c"), onDiskFilename, cflags)
	require.NoError(t, err)

	bs, err := ioutil.ReadFile(onDiskFilename)
	require.NoError(t, err)

	bundleFilename := "offset-guess.o"
	actualReader, err := bytecode.GetReader(cfg.BPFDir, bundleFilename)
	require.NoError(t, err)
	defer actualReader.Close()

	actual, err := ioutil.ReadAll(actualReader)
	require.NoError(t, err)

	assert.Equal(t, bs, actual, fmt.Sprintf("on-disk file %s and statically-linked clang compiled content %s are different", onDiskFilename, bundleFilename))
}
