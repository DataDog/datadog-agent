// +build linux_bpf

package bytecode

import (
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEbpfBytesCorrect(t *testing.T) {
	bundledFiles := map[string]string{
		"../c/tracer-ebpf.o":           "pkg/ebpf/c/tracer-ebpf.o",
		"../c/tracer-ebpf-debug.o":     "pkg/ebpf/c/tracer-ebpf-debug.o",
		"../c/oom-kill-kern.c":         "pkg/ebpf/c/oom-kill-kern.c",
		"../c/tcp-queue-length-kern.c": "pkg/ebpf/c/tcp-queue-length-kern.c",
		"../c/offset-guess.o":          "pkg/ebpf/c/offset-guess.o",
		"../c/offset-guess-debug.o":    "pkg/ebpf/c/offset-guess-debug.o",
	}

	for ondiskFilename, bundleFilename := range bundledFiles {
		bs, err := ioutil.ReadFile(ondiskFilename)
		require.NoError(t, err)

		actualReader, err := GetReader("", bundleFilename)
		require.NoError(t, err)

		actual, err := ioutil.ReadAll(actualReader)
		require.NoError(t, err)

		assert.Equal(t, bs, actual, fmt.Sprintf("on-disk file %s and bundled content %s are different", ondiskFilename, bundleFilename))
	}
}
