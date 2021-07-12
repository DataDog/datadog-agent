// +build linux freebsd openbsd darwin

package procutil

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPidExists(t *testing.T) {
	require.True(t, PidExists(os.Getpid()))
	require.False(t, PidExists(0xdeadbeef))
}
