// +build linux_bpf

package network

import (
	"context"
	"log"
	"net"
	"net/url"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
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

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := exec.CommandContext(ctx, "testdata/setup_netns.sh").Run(); err != nil {
			t.Fatalf("setup_netns.sh failed, err: %s", err)
		}
	}()

	defer func() {
		cancel()

		if err := exec.Command("testdata/teardown_netns.sh").Run(); err != nil {
			t.Errorf("failed to teardown netns: %s", err)
		}
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
	require.Eventually(t, func() bool {
		ns, err := netns.GetFromName("test")
		if err != nil {
			return false
		}

		defer ns.Close()

		nsIno, err := util.GetInoForNs(ns)
		if err != nil {
			return false
		}

		pm := NewPortMapping("/proc", true, true, testRootNs)
		err = pm.ReadInitialState()
		require.NoError(t, err)
		for _, p := range ports[:2] {
			if !pm.IsListening(p) {
				t.Errorf("pm.IsListening returned false for port %d", p)
				return false
			}
			if !pm.IsListeningWithNs(testRootNs, p) {
				t.Errorf("pm.IsListening(testRootNs) returned false for port %d", p)
				return false
			}
		}
		for _, p := range ports[2:] {
			if !pm.IsListeningWithNs(nsIno, p) {
				t.Errorf("pm.IsListening(test ns) returned false for port %d", p)
				return false
			}
		}

		if pm.IsListening(999) {
			t.Errorf("expected IsListening(999) to return false, but returned true")
			return false
		}
		if pm.IsListeningWithNs(testRootNs, 999) {
			t.Errorf("expected IsListening(testRootNs, 999) to return false, but returned true")
			return false
		}
		if pm.IsListeningWithNs(nsIno, 999) {
			t.Errorf("expected IsListening(nsIno, 999) to return false, but returned true")
			return false
		}

		return true
	}, 3*time.Second, time.Second, "tcp/tcp6 ports are listening")
}

func TestAddRemove(t *testing.T) {
	ports := NewPortMapping("/proc", true, true, testRootNs)

	require.False(t, ports.IsListening(123))
	require.False(t, ports.IsListeningWithNs(testRootNs, 123))

	ports.AddMapping(123)

	require.True(t, ports.IsListening(123))
	require.True(t, ports.IsListeningWithNs(testRootNs, 123))

	ports.RemoveMapping(123)

	require.False(t, ports.IsListening(123))
	require.False(t, ports.IsListeningWithNs(testRootNs, 123))
}

func getPort(t *testing.T, listener net.Listener) uint16 {
	addr := listener.Addr()
	listenerURL := url.URL{Scheme: addr.Network(), Host: addr.String()}
	port, err := strconv.Atoi(listenerURL.Port())
	require.NoError(t, err)
	return uint16(port)
}
