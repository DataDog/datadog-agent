package testutil

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func PingTCP(t *testing.T, ip net.IP, port int) net.Conn {
	addr := fmt.Sprintf("%s:%d", ip, port)
	network := "tcp"
	if isIpv6(ip) {
		network = "tcp6"
		addr = fmt.Sprintf("[%s]:%d", ip, port)
	}

	conn, err := net.Dial(network, addr)
	require.NoError(t, err)

	_, err = conn.Write([]byte("ping"))
	require.NoError(t, err)
	bs := make([]byte, 10)
	_, err = conn.Read(bs)
	require.NoError(t, err)

	return conn
}

func PingUDP(t *testing.T, ip net.IP, port int) net.Conn {
	network := "udp"
	if isIpv6(ip) {
		network = "udp6"
	}
	addr := &net.UDPAddr{
		IP:   ip,
		Port: port,
	}
	conn, err := net.DialUDP(network, nil, addr)
	require.NoError(t, err)

	_, err = conn.Write([]byte("ping"))
	require.NoError(t, err)

	return conn
}
