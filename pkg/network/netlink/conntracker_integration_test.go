// +build linux_bpf

package netlink

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/mdlayher/netlink"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netns"
)

const (
	natPort    = 5432
	nonNatPort = 9876
)

func TestConnTrackerCrossNamespace(t *testing.T) {
	skipUnless(t, "all_nsid")

	cmd := exec.Command("testdata/setup_cross_ns_dnat.sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		require.NoError(t, err, "setup command output %s", string(out))
	}

	defer func() {
		cmd := exec.Command("testdata/teardown_cross_ns_dnat.sh")
		if out, err := cmd.CombinedOutput(); err != nil {
			assert.NoError(t, err, "teardown command output %s", string(out))
		}
	}()

	ct, err := NewConntracker("/proc", 100, 500)
	require.NoError(t, err)
	defer ct.Close()

	time.Sleep(time.Second)

	closer := startServerTCPNs(t, net.ParseIP("2.2.2.4"), 8080, "test")
	defer closer.Close()

	laddr := pingTCP(t, net.ParseIP("2.2.2.4"), 80)

	var trans *network.IPTranslation
	require.Eventually(t, func() bool {
		trans = ct.GetTranslationForConn(
			network.ConnectionStats{
				Source: util.AddressFromNetIP(laddr.IP),
				SPort:  uint16(laddr.Port),
				Dest:   util.AddressFromString("2.2.2.4"),
				DPort:  uint16(80),
				Type:   network.TCP,
			},
		)

		if trans != nil {
			return true
		}

		return false

	}, 5*time.Second, 1*time.Second, "timed out waiting for conntrack entry")

	assert.Equal(t, uint16(8080), trans.ReplSrcPort)
}

func TestConntracker(t *testing.T) {
	cmd := exec.Command("testdata/setup_dnat.sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("setup command output: %s", string(out))
	}
	defer teardown(t)

	testConntracker(t, net.ParseIP("1.1.1.1"), net.ParseIP("2.2.2.2"))
}

func TestConntracker6(t *testing.T) {
	defer func() {
		teardown6(t)
	}()

	cmd := exec.Command("testdata/setup_dnat6.sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("setup command output: %s", string(out))
	}

	testConntracker(t, net.ParseIP("fd00::1"), net.ParseIP("fd00::2"))
}

func testConntracker(t *testing.T, serverIP, clientIP net.IP) {
	ct, err := NewConntracker("/proc", 100, 500)
	require.NoError(t, err)
	defer ct.Close()
	time.Sleep(100 * time.Millisecond)

	srv1 := startServerTCP(t, serverIP, natPort)
	defer srv1.Close()
	srv2 := startServerTCP(t, serverIP, nonNatPort)
	defer srv2.Close()
	srv3 := startServerUDP(t, serverIP, natPort)
	defer srv3.Close()

	localAddr := pingTCP(t, clientIP, natPort)
	time.Sleep(1 * time.Second)

	trans := ct.GetTranslationForConn(
		network.ConnectionStats{
			Source: util.AddressFromNetIP(localAddr.IP),
			SPort:  uint16(localAddr.Port),
			Dest:   util.AddressFromNetIP(clientIP),
			DPort:  uint16(natPort),
			Type:   network.TCP,
		},
	)
	require.NotNil(t, trans)
	assert.Equal(t, util.AddressFromNetIP(serverIP), trans.ReplSrcIP)

	localAddrUDP := pingUDP(t, clientIP, natPort).(*net.UDPAddr)
	time.Sleep(time.Second)
	trans = ct.GetTranslationForConn(
		network.ConnectionStats{
			Source: util.AddressFromNetIP(localAddrUDP.IP),
			SPort:  uint16(localAddrUDP.Port),
			Dest:   util.AddressFromNetIP(clientIP),
			DPort:  uint16(natPort),
			Type:   network.UDP,
		},
	)
	require.NotNil(t, trans)
	assert.Equal(t, util.AddressFromNetIP(serverIP), trans.ReplSrcIP)

	// now dial TCP directly
	localAddr = pingTCP(t, serverIP, nonNatPort)
	time.Sleep(time.Second)

	trans = ct.GetTranslationForConn(
		network.ConnectionStats{
			Source: util.AddressFromNetIP(localAddr.IP),
			SPort:  uint16(localAddr.Port),
			Dest:   util.AddressFromNetIP(serverIP),
			DPort:  uint16(nonNatPort),
			Type:   network.TCP,
		},
	)
	assert.Nil(t, trans)

}

// This test generates a dump of netlink messages in test_data/message_dump
// In order to execute this test, run go test with `-args netlink_dump`
func TestMessageDump(t *testing.T) {
	skipUnless(t, "netlink_dump")

	cmd := exec.Command("testdata/setup_dnat.sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("setup command output: %s", string(out))
	}
	defer teardown(t)

	f, err := os.Create("testdata/message_dump")
	require.NoError(t, err)
	defer f.Close()

	testMessageDump(t, f, net.ParseIP("1.1.1.1"), net.ParseIP("2.2.2.2"))
}

func TestMessageDump6(t *testing.T) {
	skipUnless(t, "netlink_dump")

	cmd := exec.Command("testdata/setup_dnat6.sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("setup command output: %s", string(out))
	}
	defer teardown6(t)

	f, err := os.Create("testdata/message_dump6")
	require.NoError(t, err)
	defer f.Close()

	testMessageDump(t, f, net.ParseIP("fd00::1"), net.ParseIP("fd00::2"))
}

func testMessageDump(t *testing.T, f *os.File, serverIP, clientIP net.IP) {
	consumer, err := NewConsumer("/proc", 500)
	require.NoError(t, err)
	events := consumer.Events()

	writeDone := make(chan struct{})
	go func() {
		for e := range events {
			for _, m := range e.Messages() {
				writeMsgToFile(f, m)
			}
			e.Done()
		}
		close(writeDone)
	}()

	tcpServer := startServerTCP(t, serverIP, natPort)
	defer tcpServer.Close()

	udpServer := startServerUDP(t, serverIP, nonNatPort)
	defer udpServer.Close()

	for i := 0; i < 100; i++ {
		pingTCP(t, clientIP, natPort)
		pingUDP(t, clientIP, nonNatPort)
	}

	time.Sleep(time.Second)
	consumer.Stop()
	<-writeDone
}

func skipUnless(t *testing.T, requiredArg string) {
	for _, arg := range os.Args[1:] {
		if arg == requiredArg {
			return
		}
	}

	t.Skip(
		fmt.Sprintf(
			"skipped %s. you can enable it by using running tests with `-args %s`.\n",
			t.Name(),
			requiredArg,
		),
	)
}

func writeMsgToFile(f *os.File, m netlink.Message) {
	length := make([]byte, 4)
	binary.LittleEndian.PutUint32(length, uint32(len(m.Data)))
	payload := append(length, m.Data...)
	f.Write(payload)
}

func startServerTCPNs(t *testing.T, ip net.IP, port int, ns string) io.Closer {
	h, err := netns.GetFromName(ns)
	require.NoError(t, err)

	var closer io.Closer
	util.WithNS("/proc", h, func() {
		closer = startServerTCP(t, ip, port)
	})

	return closer
}

func startServerTCP(t *testing.T, ip net.IP, port int) io.Closer {
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

func startServerUDP(t *testing.T, ip net.IP, port int) io.Closer {
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

func pingTCP(t *testing.T, ip net.IP, port int) *net.TCPAddr {
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

	return conn.LocalAddr().(*net.TCPAddr)
}

func pingUDP(t *testing.T, ip net.IP, port int) net.Addr {
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

	return conn.LocalAddr()
}

func teardown(t *testing.T) {
	cmd := exec.Command("testdata/teardown_dnat.sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("teardown command output: %s", string(out))
		t.Errorf("error tearing down: %s", err)
	}
}

func teardown6(t *testing.T) {
	cmd := exec.Command("testdata/teardown_dnat6.sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("teardown command output: %s", string(out))
		t.Errorf("error tearing down: %s", err)
	}
}

func isIpv6(ip net.IP) bool {
	return ip.To4() == nil
}
