// +build linux

package util

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProcessExists(t *testing.T) {
	require.True(t, ProcessExists(os.Getpid()))
	require.False(t, ProcessExists(0xdeadbeef))
}
