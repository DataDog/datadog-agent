// +build linux_bpf

package netlink

import (
	"fmt"
	"net"
	"os/exec"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConntracker(t *testing.T) {
	cmd := exec.Command("testdata/setup_dnat.sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("setup command output: %s", string(out))
	}

	ct, err := NewConntracker("/proc", 100, 100)
	require.NoError(t, err)
	defer ct.Close()

	<-startServerTCP(t, "1.1.1.1:5432")
	<-startServerTCP(t, "1.1.1.1:9876")
	<-startServerUDP(t, net.ParseIP("1.1.1.1"), 5432)

	localAddr := pingTCP(t, "2.2.2.2:5432")

	trans := ct.GetTranslationForConn(
		util.AddressFromNetIP(localAddr.IP), uint16(localAddr.Port),
		util.AddressFromString("2.2.2.2"), uint16(5432),
		process.ConnectionType_tcp,
	)
	require.NotNil(t, trans)
	assert.Equal(t, util.AddressFromString("1.1.1.1"), trans.ReplSrcIP)

	localAddrUDP := pingUDP(t, net.ParseIP("2.2.2.2"), 5432).(*net.UDPAddr)
	time.Sleep(time.Second)
	trans = ct.GetTranslationForConn(
		util.AddressFromNetIP(localAddrUDP.IP), uint16(localAddrUDP.Port),
		util.AddressFromString("2.2.2.2"), uint16(5432),
		process.ConnectionType_udp,
	)
	require.NotNil(t, trans)
	assert.Equal(t, util.AddressFromString("1.1.1.1"), trans.ReplSrcIP)

	// now dial TCP directly
	localAddr = pingTCP(t, "1.1.1.1:9876")
	time.Sleep(time.Second)

	trans = ct.GetTranslationForConn(
		util.AddressFromNetIP(localAddr.IP), uint16(localAddr.Port),
		util.AddressFromString("1.1.1.1"), uint16(9876),
		process.ConnectionType_tcp,
	)
	assert.Nil(t, trans)

	defer teardown(t)
}

func startServerTCP(t *testing.T, addr string) <-chan struct{} {
	ch := make(chan struct{}, 1)

	l, err := net.Listen("tcp", addr)
	require.NoError(t, err)
	go func() {
		defer l.Close()

		ch <- struct{}{}

		// serve two connections, because the test makes two TCP connections
		for i := 0; i < 2; i++ {
			conn, err := l.Accept()
			require.NoError(t, err)

			conn.Write([]byte("hello"))
			conn.Close()

		}
	}()

	return ch
}

func startServerUDP(t *testing.T, ip net.IP, port int) <-chan struct{} {
	ch := make(chan struct{}, 1)

	addr := &net.UDPAddr{
		IP:   ip,
		Port: port,
	}

	l, err := net.ListenUDP("udp", addr)
	require.NoError(t, err)
	go func() {
		defer l.Close()
		ch <- struct{}{}

		bs := make([]byte, 10)
		_, err := l.Read(bs)
		require.NoError(t, err)
	}()

	return ch
}

func pingTCP(t *testing.T, addr string) *net.TCPAddr {
	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)

	_, err = conn.Write([]byte("ping"))
	require.NoError(t, err)
	bs := make([]byte, 10)
	_, err = conn.Read(bs)
	require.NoError(t, err)

	return conn.LocalAddr().(*net.TCPAddr)
}

func pingUDP(t *testing.T, ip net.IP, port int) net.Addr {
	addr := &net.UDPAddr{
		IP:   ip,
		Port: port,
	}
	conn, err := net.DialUDP("udp", nil, addr)
	require.NoError(t, err)

	_, err = conn.Write([]byte("ping"))
	require.NoError(t, err)

	return conn.LocalAddr()
}

func teardown(t *testing.T) {
	cmd := exec.Command("testdata/teardown_dnat.sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("teardown command output: %s", string(out))
		t.Errorf("error tearing down: %s", err)
	}
}
