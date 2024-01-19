package testutil

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

// GetRandOpenPort lets the OS pick an open port and
// returns it
func GetRandOpenPort(t testing.TB) int {
	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}
