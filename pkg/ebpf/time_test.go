// +build linux

package ebpf

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNowNanoseconds(t *testing.T) {
	nn, err := NowNanoseconds()
	require.NoError(t, err)
	assert.NotZero(t, nn)
}
