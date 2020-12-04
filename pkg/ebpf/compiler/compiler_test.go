// +build linux_bpf

package compiler

import (
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompilerMatch(t *testing.T) {
	t.SkipNow()
	c, err := NewEBPFCompiler(nil, false)
	require.NoError(t, err)
	defer c.Close()

	var cflags []string
	onDiskFilename := "offset-guess-static.o"
	err = c.CompileToObjectFile("../c/offset-guess.c", onDiskFilename, cflags)
	require.NoError(t, err)

	bs, err := ioutil.ReadFile(onDiskFilename)
	require.NoError(t, err)

	bundleFilename := "offset-guess.o"
	actualReader, err := bytecode.GetReader("build", bundleFilename)
	require.NoError(t, err)

	actual, err := ioutil.ReadAll(actualReader)
	require.NoError(t, err)

	assert.Equal(t, bs, actual, fmt.Sprintf("on-disk file %s and statically-linked clang compiled content %s are different", onDiskFilename, bundleFilename))
}
