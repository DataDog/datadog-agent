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
	files := []string{"tracer-ebpf", "offset-guess"}
	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			bs, err := ioutil.ReadFile(fmt.Sprintf("../c/%s.o", f))
			require.NoError(t, err)

			actual, err := Asset(fmt.Sprintf("pkg/ebpf/c/%s.o", f))
			require.NoError(t, err)

			assert.Equal(t, bs, actual)

			bsDebug, err := ioutil.ReadFile(fmt.Sprintf("../c/%s-debug.o", f))
			require.NoError(t, err)

			actualDebug, err := Asset(fmt.Sprintf("pkg/ebpf/c/%s-debug.o", f))
			require.NoError(t, err)

			assert.Equal(t, bsDebug, actualDebug)
		})
	}
}
