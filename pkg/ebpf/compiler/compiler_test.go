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
	c := NewEBPFCompiler(true)
	defer c.Close()
	t.Logf("flags: %+v\n", c.defaultCflags)

	onDiskFilename := "../c/offset-guess-static.o"
	cflags := []string{}
	err := c.CompileToObjectFile("../c/offset-guess.c", onDiskFilename, cflags)
	require.NoError(t, err)

	bs, err := ioutil.ReadFile(onDiskFilename)
	require.NoError(t, err)

	bundleFilename := "pkg/ebpf/c/offset-guess.o"
	actualReader, err := bytecode.GetReader("", bundleFilename)
	require.NoError(t, err)

	actual, err := ioutil.ReadAll(actualReader)
	require.NoError(t, err)

	assert.Equal(t, bs, actual, fmt.Sprintf("on-disk file %s and statically-linked clang compiled content %s are different", onDiskFilename, bundleFilename))
}
