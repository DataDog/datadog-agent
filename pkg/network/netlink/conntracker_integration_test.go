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
)

func TestConntracker(t *testing.T) {
	cmd := exec.Command("testdata/setup_dnat.sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("setup command output: %s", string(out))
	}
	defer teardown(t)

	ct, err := NewConntracker("/proc", 100, 500)
	require.NoError(t, err)
	defer ct.Close()
	time.Sleep(100 * time.Millisecond)

	srv1 := startServerTCP(t, "1.1.1.1:5432")
	defer srv1.Close()
	srv2 := startServerTCP(t, "1.1.1.1:9876")
	defer srv2.Close()
	srv3 := startServerUDP(t, net.ParseIP("1.1.1.1"), 5432)
	defer srv3.Close()

	localAddr := pingTCP(t, "2.2.2.2:5432")
	time.Sleep(1 * time.Second)

	trans := ct.GetTranslationForConn(
		network.ConnectionStats{
			Source: util.AddressFromNetIP(localAddr.IP),
			SPort:  uint16(localAddr.Port),
			Dest:   util.AddressFromString("2.2.2.2"),
			DPort:  uint16(5432),
			Type:   network.TCP,
		},
	)
	require.NotNil(t, trans)
	assert.Equal(t, util.AddressFromString("1.1.1.1"), trans.ReplSrcIP)

	localAddrUDP := pingUDP(t, net.ParseIP("2.2.2.2"), 5432).(*net.UDPAddr)
	time.Sleep(time.Second)
	trans = ct.GetTranslationForConn(
		network.ConnectionStats{
			Source: util.AddressFromNetIP(localAddrUDP.IP),
			SPort:  uint16(localAddrUDP.Port),
			Dest:   util.AddressFromString("2.2.2.2"),
			DPort:  uint16(5432),
			Type:   network.UDP,
		},
	)
	require.NotNil(t, trans)
	assert.Equal(t, util.AddressFromString("1.1.1.1"), trans.ReplSrcIP)

	// now dial TCP directly
	localAddr = pingTCP(t, "1.1.1.1:9876")
	time.Sleep(time.Second)

	trans = ct.GetTranslationForConn(
		network.ConnectionStats{
			Source: util.AddressFromNetIP(localAddr.IP),
			SPort:  uint16(localAddr.Port),
			Dest:   util.AddressFromString("1.1.1.1"),
			DPort:  uint16(9876),
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

	consumer, err := NewConsumer("/proc", 500)
	require.NoError(t, err)
	events := consumer.Events()

	f, err := os.Create("testdata/message_dump")
	require.NoError(t, err)
	defer f.Close()

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

	tcpServer := startServerTCP(t, "1.1.1.1:5432")
	defer tcpServer.Close()

	udpServer := startServerUDP(t, net.ParseIP("1.1.1.1"), 9876)
	defer udpServer.Close()

	for i := 0; i < 100; i++ {
		pingTCP(t, "2.2.2.2:5432")
		pingUDP(t, net.ParseIP("2.2.2.2"), 9876)
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

func startServerTCP(t *testing.T, addr string) io.Closer {
	ch := make(chan struct{})

	l, err := net.Listen("tcp", addr)
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

	addr := &net.UDPAddr{
		IP:   ip,
		Port: port,
	}

	l, err := net.ListenUDP("udp", addr)
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
