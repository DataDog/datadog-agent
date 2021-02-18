package testutil

import (
	"fmt"
	"io"
	"net"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netns"
)

func StartServerTCPNs(t *testing.T, ip net.IP, port int, ns string) io.Closer {
	h, err := netns.GetFromName(ns)
	require.NoError(t, err)

	var closer io.Closer
	util.WithNS("/proc", h, func() error {
		closer = StartServerTCP(t, ip, port)
		return nil
	})

	return closer
}

func StartServerTCP(t *testing.T, ip net.IP, port int) io.Closer {
	ch := make(chan struct{})
	addr := fmt.Sprintf("%s:%d", ip, port)
	network := "tcp"
	if isIpv6(ip) {
		network = "tcp6"
		addr = fmt.Sprintf("[%s]:%d", ip, port)
	}

	l, err := net.Listen(network, addr)
	require.NoError(t, err)
	go func() {
		close(ch)
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}

			conn.Write([]byte("hello"))
			conn.Close()
		}
	}()
	<-ch

	return l
}

func StartServerUDPNs(t *testing.T, ip net.IP, port int, ns string) io.Closer {
	h, err := netns.GetFromName(ns)
	require.NoError(t, err)

	var closer io.Closer
	util.WithNS("/proc", h, func() error {
		closer = StartServerUDP(t, ip, port)
		return nil
	})

	return closer
}

func StartServerUDP(t *testing.T, ip net.IP, port int) io.Closer {
	ch := make(chan struct{})
	network := "udp"
	if isIpv6(ip) {
		network = "udp6"
	}

	addr := &net.UDPAddr{
		IP:   ip,
		Port: port,
	}

	l, err := net.ListenUDP(network, addr)
	require.NoError(t, err)
	go func() {
		close(ch)

		for {
			bs := make([]byte, 10)
			_, err := l.Read(bs)
			if err != nil {
				return
			}
		}
	}()
	<-ch

	return l
}

func isIpv6(ip net.IP) bool {
	return ip.To4() == nil
}
