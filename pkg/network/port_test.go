// +build linux

package network

import (
	"net"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadInitialState(t *testing.T) {
	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }()

	l6, err := net.Listen("tcp6", ":0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }()

	tcpPort := getPort(t, l)
	tcp6Port := getPort(t, l6)

	ports := NewPortMapping("/proc", NewDefaultConfig())

	err = ports.ReadInitialState()
	require.NoError(t, err)

	require.True(t, ports.IsListening(tcpPort))
	require.True(t, ports.IsListening(tcp6Port))

	require.False(t, ports.IsListening(999))
}

func TestAddRemove(t *testing.T) {
	ports := NewPortMapping("/proc", NewDefaultConfig())

	require.False(t, ports.IsListening(123))

	ports.AddMapping(123)

	require.True(t, ports.IsListening(123))

	ports.RemoveMapping(123)

	require.False(t, ports.IsListening(123))
}

func getPort(t *testing.T, listener net.Listener) uint16 {
	addr := listener.Addr()
	listenerURL := url.URL{Scheme: addr.Network(), Host: addr.String()}
	port, err := strconv.Atoi(listenerURL.Port())
	require.NoError(t, err)
	return uint16(port)
}
