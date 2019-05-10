package ebpf

import (
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEbpfBytesCorrect(t *testing.T) {
	// TODO(sami): remove this once object files are back
	t.SkipNow()
	bs, err := ioutil.ReadFile("c/tracer-ebpf.o")
	require.NoError(t, err)

	actual, err := tracerEbpfOBytes()
	require.NoError(t, err)

	assert.Equal(t, bs, actual)

	bsDebug, err := ioutil.ReadFile("c/tracer-ebpf-debug.o")
	require.NoError(t, err)

	actualDebug, err := tracerEbpfDebugOBytes()
	require.NoError(t, err)

	assert.Equal(t, bsDebug, actualDebug)
}
