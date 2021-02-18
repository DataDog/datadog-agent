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
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/testutil"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/mdlayher/netlink"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	natPort    = 5432
	nonNatPort = 9876
)

func TestConnTrackerCrossNamespaceAllNsEnabled(t *testing.T) {
	cmd := exec.Command("testdata/setup_cross_ns_dnat.sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		require.NoError(t, err, "setup command output %s", string(out))
	}

	defer teardownCrossNs(t)

	ct, closer, laddr := setupTestConnTrackerCrossNamespace(t, true)
	defer ct.Close()
	defer closer.Close()

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

func TestConnTrackerCrossNamespaceAllNsDisabled(t *testing.T) {
	cmd := exec.Command("testdata/setup_cross_ns_dnat.sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		require.NoError(t, err, "setup command output %s", string(out))
	}

	defer teardownCrossNs(t)

	ct, closer, laddr := setupTestConnTrackerCrossNamespace(t, false)
	defer ct.Close()
	defer closer.Close()

	time.Sleep(time.Second)
	trans := ct.GetTranslationForConn(
		network.ConnectionStats{
			Source: util.AddressFromNetIP(laddr.IP),
			SPort:  uint16(laddr.Port),
			Dest:   util.AddressFromString("2.2.2.4"),
			DPort:  uint16(80),
			Type:   network.TCP,
		},
	)

	assert.Nil(t, trans)
}

func setupTestConnTrackerCrossNamespace(t *testing.T, enableAllNs bool) (Conntracker, io.Closer, *net.TCPAddr) {
	cfg := config.NewDefaultConfig()
	cfg.ProcRoot = "/proc"
	cfg.ConntrackMaxStateSize = 100
	cfg.ConntrackRateLimit = 500
	cfg.EnableConntrackAllNamespaces = enableAllNs
	ct, err := NewConntracker(cfg)
	require.NoError(t, err)

	time.Sleep(time.Second)

	closer := testutil.StartServerTCPNs(t, net.ParseIP("2.2.2.4"), 8080, "test")

	laddr := testutil.PingTCP(t, net.ParseIP("2.2.2.4"), 80).LocalAddr().(*net.TCPAddr)
	return ct, closer, laddr
}

func TestConntracker(t *testing.T) {
	cmd := exec.Command("testdata/setup_dnat.sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("setup command output: %s", string(out))
	}
	defer teardown(t)

	ct, err := NewConntracker("/proc", 100, 500, false)
	require.NoError(t, err)
	defer ct.Close()
	time.Sleep(100 * time.Millisecond)
	testutil.TestConntracker(t, net.ParseIP("1.1.1.1"), net.ParseIP("2.2.2.2"), ct)
}

func TestConntracker6(t *testing.T) {
	defer func() {
		teardown6(t)
	}()

	cmd := exec.Command("testdata/setup_dnat6.sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("setup command output: %s", string(out))
	}

	cfg := config.NewDefaultConfig()
	cfg.ProcRoot = "/proc"
	cfg.ConntrackMaxStateSize = 100
	cfg.ConntrackRateLimit = 500
	cfg.EnableConntrackAllNamespaces = false
	ct, err := NewConntracker(cfg)
	require.NoError(t, err)
	defer ct.Close()
	time.Sleep(100 * time.Millisecond)
	testutil.TestConntracker(t, net.ParseIP("fd00::1"), net.ParseIP("fd00::2"), ct)
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
	consumer := NewConsumer("/proc", 500, false)
	events, err := consumer.Events()
	require.NoError(t, err)

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

	tcpServer := testutil.StartServerTCP(t, serverIP, natPort)
	defer tcpServer.Close()

	udpServer := testutil.StartServerUDP(t, serverIP, nonNatPort)
	defer udpServer.Close()

	for i := 0; i < 100; i++ {
		testutil.PingTCP(t, clientIP, natPort)
		testutil.PingUDP(t, clientIP, nonNatPort)
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

func teardownCrossNs(t *testing.T) {
	cmd := exec.Command("testdata/teardown_cross_ns_dnat.sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		assert.NoError(t, err, "teardown command output %s", string(out))
	}
}
