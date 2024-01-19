package testutil

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func GetRandOpenPort(t testing.TB) (int){
	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

