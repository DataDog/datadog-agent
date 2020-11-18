// +build linux_bpf

package compiler

import (
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompilerMatch(t *testing.T) {
	c := NewEBPFCompiler(true)
	defer c.Close()
	t.Logf("flags: %+v\n", c.defaultCflags)

	cflags := []string{
		"-DCONFIG_64BIT",
		"-D__BPF_TRACING__",
		`-DKBUILD_MODNAME=ddsysprobe`,
		"-Wno-unused-value",
		"-Wno-pointer-sign",
		"-Wno-compare-distinct-pointer-types",
		"-Wunused",
		"-Wall",
		"-Werror",
		"-include../c/asm_goto_workaround.h",
		"-resource-dir=/opt/clang/lib/clang/11.0.0",
	}

	dirs, err := kernel.FindHeaderDirs()
	require.NoError(t, err, "unable to find kernel headers")

	for _, d := range dirs {
		cflags = append(cflags,
			fmt.Sprintf("-isystem%s/arch/x86/include", d),
			fmt.Sprintf("-isystem%s/arch/x86/include/generated", d),
			fmt.Sprintf("-isystem%s/include", d),
			fmt.Sprintf("-isystem%s/arch/x86/include/uapi", d),
			fmt.Sprintf("-isystem%s/arch/x86/include/generated/uapi", d),
			fmt.Sprintf("-isystem%s/include/uapi", d),
			fmt.Sprintf("-isystem%s/include/generated/uapi", d),
		)
	}

	onDiskFilename := "../c/offset-guess-static.o"
	err = c.CompileToObjectFile("../c/offset-guess.c", onDiskFilename, cflags)
	require.NoError(t, err)

	bs, err := ioutil.ReadFile(onDiskFilename)
	require.NoError(t, err)

	bundleFilename := "pkg/ebpf/c/offset-guess.o"
	actualReader, err := bytecode.GetReader("../c", bundleFilename)
	require.NoError(t, err)

	actual, err := ioutil.ReadAll(actualReader)
	require.NoError(t, err)

	assert.Equal(t, bs, actual, fmt.Sprintf("on-disk file %s and statically-linked clang compiled content %s are different", onDiskFilename, bundleFilename))
}
