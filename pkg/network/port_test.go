// +build linux_bpf

package network

import (
	"log"
	"net"
	"net/url"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netns"
)

var testRootNs uint64

func TestMain(m *testing.M) {
	rootNs, err := util.GetRootNetNamespace("/proc")
	if err != nil {
		log.Fatal(err)
	}
	testRootNs, err = util.GetInoForNs(rootNs)
	if err != nil {
		log.Fatal(err)
	}

	m.Run()
}

func TestReadInitialState(t *testing.T) {

	err := exec.Command("testdata/setup_netns.sh").Run()
	require.NoError(t, err, "setup_netns.sh failed")

	defer func() {
		err := exec.Command("testdata/teardown_netns.sh").Run()
		assert.NoError(t, err, "failed to teardown netns")
	}()

	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }()

	l6, err := net.Listen("tcp6", ":0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }()

	ports := []uint16{
		getPort(t, l),
		getPort(t, l6),
		34567,
		34568,
	}

	ns, err := netns.GetFromName("test")
	require.NoError(t, err)
	defer ns.Close()

	nsIno, err := util.GetInoForNs(ns)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		pm := NewPortMapping("/proc", true, true)
		err = pm.ReadInitialState()
		require.NoError(t, err)
		for _, p := range ports[:2] {
			if !pm.IsListening(testRootNs, p) {
				t.Errorf("pm.IsListening(testRootNs) returned false for port %d", p)
				return false
			}
		}
		for _, p := range ports[2:] {
			if !pm.IsListening(nsIno, p) {
				t.Errorf("pm.IsListening(test ns) returned false for port %d", p)
				return false
			}
		}

		if pm.IsListening(testRootNs, 999) {
			t.Errorf("expected IsListening(testRootNs, 999) to return false, but returned true")
			return false
		}
		if pm.IsListening(nsIno, 999) {
			t.Errorf("expected IsListening(nsIno, 999) to return false, but returned true")
			return false
		}

		return true
	}, 3*time.Second, time.Second, "tcp/tcp6 ports are listening")
}

func TestReadInitialUDPState(t *testing.T) {

	err := exec.Command("testdata/setup_netns.sh").Run()
	require.NoError(t, err, "setup_netns.sh failed")

	defer func() {
		err := exec.Command("testdata/teardown_netns.sh").Run()
		assert.NoError(t, err, "failed to teardown netns")
	}()

	l, err := net.ListenUDP("udp", &net.UDPAddr{})
	require.NoError(t, err)
	defer func() { _ = l.Close() }()

	l6, err := net.ListenUDP("udp6", &net.UDPAddr{})
	require.NoError(t, err)
	defer func() { _ = l.Close() }()

	ports := []uint16{
		getPortUDP(t, l),
		getPortUDP(t, l6),
		34567,
		34568,
	}

	ns, err := netns.GetFromName("test")
	require.NoError(t, err)
	defer ns.Close()

	_, err = util.GetInoForNs(ns)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		pm := NewPortMapping("/proc", true, true)
		err = pm.ReadInitialUDPState()
		require.NoError(t, err)
		for _, p := range ports {
			if !pm.IsListening(0, p) {
				t.Errorf("pm.IsListening(0, p) returned false for port %d", p)
				return false
			}
		}

		if pm.IsListening(0, 999) {
			t.Errorf("expected IsListening(0, 999) to return false, but returned true")
			return false
		}

		return true
	}, 3*time.Second, time.Second, "udp/udp6 ports are listening")
}

func TestAddRemove(t *testing.T) {
	ports := NewPortMapping("/proc", true, true)

	const testNs uint64 = 1234

	require.False(t, ports.IsListening(testNs, 123))

	ports.AddMapping(testNs, 123)

	require.True(t, ports.IsListening(testNs, 123))

	ports.RemoveMapping(testNs, 123)

	require.False(t, ports.IsListening(testNs, 123))
}

func getPort(t *testing.T, listener net.Listener) uint16 {
	addr := listener.Addr()
	listenerURL := url.URL{Scheme: addr.Network(), Host: addr.String()}
	port, err := strconv.Atoi(listenerURL.Port())
	require.NoError(t, err)
	return uint16(port)
}

func getPortUDP(_ *testing.T, udpConn *net.UDPConn) uint16 {
	return uint16(udpConn.LocalAddr().(*net.UDPAddr).Port)
}
