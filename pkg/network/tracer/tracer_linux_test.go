// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package tracer

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/features"
	"github.com/cilium/ebpf/rlimit"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	vnetns "github.com/vishvananda/netns"
	"go4.org/intern"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/config/sysctl"
	"github.com/DataDog/datadog-agent/pkg/network/events"
	netlinktestutil "github.com/DataDog/datadog-agent/pkg/network/netlink/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/offsetguess"
	tracertestutil "github.com/DataDog/datadog-agent/pkg/network/tracer/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/testdns"
	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
)

var kv470 = kernel.VersionCode(4, 7, 0)
var kv = kernel.MustHostVersion()

func platformInit() {
	// linux-specific tasks here
}

func (s *TracerSuite) TestTCPRemoveEntries() {
	t := s.T()
	config := testConfig()
	config.TCPConnTimeout = 100 * time.Millisecond
	tr := setupTracer(t, config)
	// Create a dummy TCP Server
	server := tracertestutil.NewTCPServer(func(_ net.Conn) {
	})
	t.Cleanup(server.Shutdown)
	require.NoError(t, server.Run())

	// Connect to server
	c, err := net.DialTimeout("tcp", server.Address(), 2*time.Second)
	require.NoError(t, err)
	defer c.Close()

	// Write a message
	_, err = c.Write(genPayload(clientMessageSize))
	require.NoError(t, err)

	// Write a bunch of messages with blocking iptable rule to create retransmits
	iptablesWrapper(t, func() {
		for i := 0; i < 99; i++ {
			// Send a bunch of messages
			c.Write(genPayload(clientMessageSize))
		}
		time.Sleep(time.Second)
	})

	c.Close()

	// Create a new client
	c2, err := net.DialTimeout("tcp", server.Address(), 1*time.Second)
	require.NoError(t, err)

	// Send a messages
	_, err = c2.Write(genPayload(clientMessageSize))
	require.NoError(t, err)
	defer c2.Close()

	conn, ok := findConnection(c2.LocalAddr(), c2.RemoteAddr(), getConnections(t, tr))
	require.True(t, ok)
	assert.Equal(t, clientMessageSize, int(conn.Monotonic.SentBytes))
	assert.Equal(t, 0, int(conn.Monotonic.RecvBytes))
	assert.Equal(t, 0, int(conn.Monotonic.Retransmits))
	assert.Equal(t, os.Getpid(), int(conn.Pid))
	assert.Equal(t, addrPort(server.Address()), int(conn.DPort))

	// Make sure the first connection got cleaned up
	assert.Eventually(t, func() bool {
		_, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), getConnections(t, tr))
		return !ok
	}, 5*time.Second, 100*time.Millisecond)

}

func (s *TracerSuite) TestTCPRetransmit() {
	t := s.T()
	// Enable BPF-based system probe
	tr := setupTracer(t, testConfig())

	// Create TCP Server which sends back serverMessageSize bytes
	server := tracertestutil.NewTCPServer(func(c net.Conn) {
		r := bufio.NewReader(c)
		r.ReadBytes(byte('\n'))
		c.Write(genPayload(serverMessageSize))
		c.Close()
	})
	t.Cleanup(server.Shutdown)
	require.NoError(t, server.Run())

	// Connect to server
	c, err := net.DialTimeout("tcp", server.Address(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Write clientMessageSize to server, and read response
	if _, err = c.Write(genPayload(clientMessageSize)); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(c)
	r.ReadBytes(byte('\n'))

	iptablesWrapper(t, func() {
		for i := 0; i < 99; i++ {
			// Send a bunch of messages
			c.Write(genPayload(clientMessageSize))
		}
		time.Sleep(time.Second)
	})

	// Iterate through active connections until we find connection created above, and confirm send + recv counts and there was at least 1 retransmission
	connections := getConnections(t, tr)

	conn, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	require.True(t, ok)

	assert.Equal(t, 100*clientMessageSize, int(conn.Monotonic.SentBytes))
	assert.True(t, int(conn.Monotonic.Retransmits) > 0)
	assert.Equal(t, os.Getpid(), int(conn.Pid))
	assert.Equal(t, addrPort(server.Address()), int(conn.DPort))
}

func (s *TracerSuite) TestTCPRetransmitSharedSocket() {
	t := s.T()
	// Create TCP Server that simply "drains" connection until receiving an EOF
	server := tracertestutil.NewTCPServer(func(c net.Conn) {
		io.Copy(io.Discard, c)
		c.Close()
	})
	t.Cleanup(server.Shutdown)
	require.NoError(t, server.Run())

	// Connect to server
	c, err := net.DialTimeout("tcp", server.Address(), time.Second)
	require.NoError(t, err)
	defer c.Close()

	socketFile, err := c.(*net.TCPConn).File()
	require.NoError(t, err)
	defer socketFile.Close()

	// Enable BPF-based system probe.
	// normally this is done first thing in a test
	// to collect all test traffic, but
	// this is done late here so that the server
	// incoming/outgoing connection is not recorded.
	// if this connection is recorded, it can lead
	// to 11 connections being reported below instead
	// of 10, since tcp stats can get attached to
	// this connection (if there are pid collisions,
	// we assign the tcp stats to one connection randomly,
	// which is the point of this test)
	tr := setupTracer(t, testConfig())

	const numProcesses = 10
	iptablesWrapper(t, func() {
		for i := 0; i < numProcesses; i++ {
			// Establish one connection per process, all sharing the same socket represented by fd=3
			// https://github.com/golang/go/blob/release-branch.go1.10/src/os/exec/exec.go#L111-L114
			msg := genPayload(clientMessageSize)
			cmd := exec.Command("bash", "-c", fmt.Sprintf("echo -ne %q >&3", msg))
			cmd.ExtraFiles = []*os.File{socketFile}
			err := cmd.Run()
			require.NoError(t, err)
		}
		time.Sleep(time.Second)
	})

	// Fetch all connections matching source and target address
	allConnections := getConnections(t, tr)
	conns := network.FilterConnections(allConnections, network.ByTuple(c.LocalAddr(), c.RemoteAddr()))
	require.Len(t, conns, numProcesses)

	totalSent := 0
	for _, c := range conns {
		totalSent += int(c.Monotonic.SentBytes)
	}
	assert.Equal(t, numProcesses*clientMessageSize, totalSent)

	// Since we can't reliably identify the PID associated to a retransmit, we have opted
	// to report the total number of retransmits for *one* of the connections sharing the
	// same socket
	connsWithRetransmits := 0
	for _, c := range conns {
		if c.Monotonic.Retransmits > 0 {
			connsWithRetransmits++
		}
	}
	assert.Equal(t, 1, connsWithRetransmits)

	// Test if telemetry measuring PID collisions is correct
	// >= because there can be other connections going on during CI that increase pidCollisions
	assert.GreaterOrEqual(t, connection.EbpfTracerTelemetry.PidCollisions.Load(), int64(numProcesses-1))
}

func (s *TracerSuite) TestTCPRTT() {
	t := s.T()
	// mark as flaky since the offset for RTT can be incorrectly guessed on prebuilt
	if ebpftest.GetBuildMode() == ebpftest.Prebuilt {
		flake.Mark(t)
	}
	// Enable BPF-based system probe
	tr := setupTracer(t, testConfig())
	// Create TCP Server that simply "drains" connection until receiving an EOF
	server := tracertestutil.NewTCPServer(func(c net.Conn) {
		io.Copy(io.Discard, c)
		c.Close()
	})
	t.Cleanup(server.Shutdown)
	require.NoError(t, server.Run())

	c, err := net.DialTimeout("tcp", server.Address(), time.Second)
	require.NoError(t, err)
	defer c.Close()

	// Wait for a second so RTT can stabilize
	time.Sleep(1 * time.Second)

	// Write something to socket to ensure connection is tracked
	// This will trigger the collection of TCP stats including RTT
	_, err = c.Write([]byte("foo"))
	require.NoError(t, err)

	// Obtain information from a TCP socket via GETSOCKOPT(2) system call.
	tcpInfo, err := offsetguess.TcpGetInfo(c)
	require.NoError(t, err)

	// Fetch connection matching source and target address
	allConnections := getConnections(t, tr)
	conn, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), allConnections)
	require.True(t, ok)

	// Assert that values returned from syscall match ones generated by eBPF program
	assert.EqualValues(t, int(tcpInfo.Rtt), int(conn.RTT))
	assert.EqualValues(t, int(tcpInfo.Rttvar), int(conn.RTTVar))
}

func (s *TracerSuite) TestTCPMiscount() {
	t := s.T()
	t.Skip("skipping because this test will pass/fail depending on host performance")
	tr := setupTracer(t, testConfig())
	// Create a dummy TCP Server
	server := tracertestutil.NewTCPServer(func(c net.Conn) {
		r := bufio.NewReader(c)
		for {
			if _, err := r.ReadBytes(byte('\n')); err != nil { // indicates that EOF has been reached,
				break
			}
		}
		c.Close()
	})
	t.Cleanup(server.Shutdown)
	require.NoError(t, server.Run())

	c, err := net.DialTimeout("tcp", server.Address(), 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	file, err := c.(*net.TCPConn).File()
	require.NoError(t, err)

	fd := int(file.Fd())

	// Set a really low sendtimeout of 1us to trigger EAGAIN errors in `tcp_sendmsg`
	err = syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_SNDTIMEO, &syscall.Timeval{
		Sec:  0,
		Usec: 1,
	})
	require.NoError(t, err)

	// 100 MB payload
	x := make([]byte, 100*1024*1024)

	n, err := c.Write(x)
	assert.NoError(t, err)
	assert.EqualValues(t, len(x), n)

	server.Shutdown()

	conn, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), getConnections(t, tr))
	if assert.True(t, ok) {
		// TODO this should not happen but is expected for now
		// we don't have the correct count since retries happened
		assert.False(t, uint64(len(x)) == conn.Monotonic.SentBytes)
	}

	assert.NotZero(t, connection.EbpfTracerTelemetry.LastTcpSentMiscounts.Load())
}

func (s *TracerSuite) TestConnectionExpirationRegression() {
	t := s.T()
	t.SkipNow()
	tr := setupTracer(t, testConfig())
	// Create TCP Server that simply "drains" connection until receiving an EOF
	connClosed := make(chan struct{})
	server := tracertestutil.NewTCPServer(func(c net.Conn) {
		io.Copy(io.Discard, c)
		c.Close()
		connClosed <- struct{}{}
	})
	t.Cleanup(server.Shutdown)
	require.NoError(t, server.Run())

	c, err := net.DialTimeout("tcp", server.Address(), time.Second)
	require.NoError(t, err)

	// Write 5 bytes to TCP socket
	payload := []byte("12345")
	_, err = c.Write(payload)
	require.NoError(t, err)

	// Fetch connection matching source and target address
	// This will make sure to populate the state for this particular client
	allConnections := getConnections(t, tr)
	connectionStats, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), allConnections)
	require.True(t, ok)
	assert.Equal(t, uint64(len(payload)), connectionStats.Last.SentBytes)

	// This emulates the race condition, a `tcp_close` followed by a call to `Tracer.removeConnections()`
	// It's unfortunate we're relying here on private methods, but there isn't much we can do to avoid that.
	c.Close()
	<-connClosed
	time.Sleep(100 * time.Millisecond)
	tr.ebpfTracer.Remove(connectionStats)

	// Since no bytes were send or received after we obtained the connectionStats, we should have 0 LastBytesSent
	allConnections = getConnections(t, tr)
	connectionStats, ok = findConnection(c.LocalAddr(), c.RemoteAddr(), allConnections)
	require.True(t, ok)
	assert.Equal(t, uint64(0), connectionStats.Last.SentBytes)

	// Finally, this connection should have been expired from the state
	allConnections = getConnections(t, tr)
	_, ok = findConnection(c.LocalAddr(), c.RemoteAddr(), allConnections)
	require.False(t, ok)
}

func (s *TracerSuite) TestConntrackExpiration() {
	t := s.T()
	ebpftest.LogLevel(t, "trace")

	cfg := testConfig()
	skipOnEbpflessNotSupported(t, cfg)
	netlinktestutil.SetupDNAT(t)

	tr := setupTracer(t, testConfig())

	server := tracertestutil.NewTCPServerOnAddress("1.1.1.1:0", func(c net.Conn) {
		defer c.Close()

		r := bufio.NewReader(c)
		for {
			b, err := r.ReadBytes(byte('\n'))
			if err != nil {
				if err == io.EOF {
					return
				}
				require.NoError(t, err)
			}
			if len(b) == 0 {
				return
			}
		}
	})
	require.NoError(t, server.Run())
	t.Cleanup(server.Shutdown)

	_, port, err := net.SplitHostPort(server.Address())
	require.NoError(t, err, "could not split server address %s", server.Address())

	c, err := net.Dial("tcp", "2.2.2.2:"+port)
	require.NoError(t, err)
	t.Cleanup(func() {
		c.Close()
	})

	var conn *network.ConnectionStats
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		_, err = c.Write([]byte("ping\n"))
		if !assert.NoError(collect, err, "error sending data to server") {
			return
		}

		connections := getConnections(t, tr)
		t.Log(connections) // for debugging failures
		var ok bool
		conn, ok = findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
		if !assert.True(collect, ok, "connection not found") {
			return
		}
		assert.NotNil(collect, tr.conntracker.GetTranslationForConn(&conn.ConnectionTuple), "connection does not have NAT translation")
	}, 3*time.Second, 100*time.Millisecond, "failed to find connection translation")

	// This will force the connection to be expired next time we call getConnections, but
	// conntrack should still have the connection information since the connection is still
	// alive
	tr.config.TCPConnTimeout = time.Duration(-1)
	_ = getConnections(t, tr)

	assert.NotNil(t, tr.conntracker.GetTranslationForConn(&conn.ConnectionTuple), "translation should not have been deleted")

	// delete the connection from system conntrack
	cmd := exec.Command("conntrack", "-D", "-s", c.LocalAddr().(*net.TCPAddr).IP.String(), "-d", c.RemoteAddr().(*net.TCPAddr).IP.String(), "-p", "tcp")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "conntrack delete failed, output: %s", out)
	_ = getConnections(t, tr)

	assert.Nil(t, tr.conntracker.GetTranslationForConn(&conn.ConnectionTuple), "translation should have been deleted")

	// write newline so server connections will exit
	_, err = c.Write([]byte("\n"))
	require.NoError(t, err)
}

// This test ensures that conntrack lookups are retried for short-lived
// connections when the first lookup fails
func (s *TracerSuite) TestConntrackDelays() {
	t := s.T()
	netlinktestutil.SetupDNAT(t)
	wg := sync.WaitGroup{}

	tr := setupTracer(t, testConfig())
	// This will ensure that the first lookup for every connection fails, while the following ones succeed
	tr.conntracker = tracertestutil.NewDelayedConntracker(tr.conntracker, 1)

	// Letting the OS pick an open port is necessary to avoid flakiness in the test. Running the the test multiple
	// times can fail if binding to the same port since Conntrack might not emit NEW events for the same tuple
	server := tracertestutil.NewTCPServerOnAddress(fmt.Sprintf("1.1.1.1:%d", 0), func(c net.Conn) {
		wg.Add(1)
		defer wg.Done()
		defer c.Close()

		r := bufio.NewReader(c)
		r.ReadBytes(byte('\n'))
	})
	t.Cleanup(server.Shutdown)
	require.NoError(t, server.Run())

	_, port, err := net.SplitHostPort(server.Address())
	require.NoError(t, err)
	c, err := net.Dial("tcp", fmt.Sprintf("2.2.2.2:%s", port))
	require.NoError(t, err)
	defer c.Close()
	_, err = c.Write([]byte("ping"))
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		connections := getConnections(t, tr)
		conn, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
		return ok && tr.conntracker.GetTranslationForConn(&conn.ConnectionTuple) != nil
	}, 3*time.Second, 100*time.Millisecond, "failed to find connection with translation")

	// write newline so server connections will exit
	_, err = c.Write([]byte("\n"))
	require.NoError(t, err)
	wg.Wait()
}

func (s *TracerSuite) TestTranslationBindingRegression() {
	t := s.T()
	netlinktestutil.SetupDNAT(t)
	wg := sync.WaitGroup{}

	tr := setupTracer(t, testConfig())

	// Setup TCP server
	server := tracertestutil.NewTCPServerOnAddress(fmt.Sprintf("1.1.1.1:%d", 0), func(c net.Conn) {
		wg.Add(1)
		defer wg.Done()
		defer c.Close()

		r := bufio.NewReader(c)
		r.ReadBytes(byte('\n'))
	})
	t.Cleanup(server.Shutdown)
	require.NoError(t, server.Run())

	// Send data to 2.2.2.2 (which should be translated to 1.1.1.1)
	_, port, err := net.SplitHostPort(server.Address())
	require.NoError(t, err)
	c, err := net.Dial("tcp", fmt.Sprintf("2.2.2.2:%s", port))
	require.NoError(t, err)
	defer c.Close()
	_, err = c.Write([]byte("ping"))
	require.NoError(t, err)

	// wait for conntrack update
	laddr := c.LocalAddr().(*net.TCPAddr)
	raddr := c.RemoteAddr().(*net.TCPAddr)
	cs := network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
		DPort:  uint16(raddr.Port),
		Dest:   util.AddressFromNetIP(raddr.IP),
		Family: network.AFINET,
		SPort:  uint16(laddr.Port),
		Source: util.AddressFromNetIP(laddr.IP),
		Type:   network.TCP,
	}}
	require.Eventually(t, func() bool {
		return tr.conntracker.GetTranslationForConn(&cs.ConnectionTuple) != nil
	}, 3*time.Second, 100*time.Millisecond, "timed out waiting for conntrack update")

	// Assert that the connection to 2.2.2.2 has an IPTranslation object bound to it
	connections := getConnections(t, tr)
	conn, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	require.True(t, ok)
	require.NotNil(t, conn.IPTranslation, "missing translation for connection")

	// write newline so server connections will exit
	_, err = c.Write([]byte("\n"))
	require.NoError(t, err)
	wg.Wait()
}

func (s *TracerSuite) TestUnconnectedUDPSendIPv6() {
	t := s.T()
	cfg := testConfig()
	if !cfg.CollectUDPv6Conns {
		t.Skip("UDPv6 disabled")
	}

	tr := setupTracer(t, cfg)
	linkLocal, err := offsetguess.GetIPv6LinkLocalAddress()
	require.NoError(t, err)

	remotePort := rand.Int()%5000 + 15000
	remoteAddr := &net.UDPAddr{IP: net.ParseIP(offsetguess.InterfaceLocalMulticastIPv6), Port: remotePort}
	conn, err := net.ListenUDP("udp6", linkLocal[0])
	require.NoError(t, err)
	defer conn.Close()
	message := []byte("payload")
	bytesSent, err := conn.WriteTo(message, remoteAddr)
	require.NoError(t, err)

	connections := getConnections(t, tr)
	outgoing := network.FilterConnections(connections, func(cs network.ConnectionStats) bool {
		return cs.DPort == uint16(remotePort)
	})

	require.Len(t, outgoing, 1)
	assert.Equal(t, remoteAddr.IP.String(), outgoing[0].Dest.String())
	assert.Equal(t, bytesSent, int(outgoing[0].Monotonic.SentBytes))
}

func (s *TracerSuite) TestGatewayLookupNotEnabled() {
	t := s.T()
	t.Run("gateway lookup enabled, not on aws", func(t *testing.T) {
		cfg := testConfig()
		cfg.EnableGatewayLookup = true
		oldCloud := network.Cloud
		defer func() {
			network.Cloud = oldCloud
		}()
		ctrl := gomock.NewController(t)
		m := NewMockcloudProvider(ctrl)
		m.EXPECT().IsAWS().Return(false)
		network.Cloud = m
		tr := setupTracer(t, cfg)
		require.Nil(t, tr.gwLookup)
	})

	t.Run("gateway lookup enabled, aws metadata endpoint not enabled", func(t *testing.T) {
		cfg := testConfig()
		cfg.EnableGatewayLookup = true
		oldCloud := network.Cloud
		defer func() {
			network.Cloud = oldCloud
		}()
		ctrl := gomock.NewController(t)
		m := NewMockcloudProvider(ctrl)
		m.EXPECT().IsAWS().Return(true)
		network.Cloud = m

		clouds := pkgconfigsetup.Datadog().Get("cloud_provider_metadata")
		pkgconfigsetup.Datadog().SetWithoutSource("cloud_provider_metadata", []string{})
		defer pkgconfigsetup.Datadog().SetWithoutSource("cloud_provider_metadata", clouds)

		tr := setupTracer(t, cfg)
		require.Nil(t, tr.gwLookup)
	})
}

func (s *TracerSuite) TestGatewayLookupEnabled() {
	t := s.T()
	ctrl := gomock.NewController(t)
	m := NewMockcloudProvider(ctrl)
	oldCloud := network.Cloud
	defer func() {
		network.Cloud = oldCloud
	}()

	m.EXPECT().IsAWS().Return(true)
	network.Cloud = m

	dnsAddr := net.ParseIP("8.8.8.8")
	ifi := ipRouteGet(t, "", dnsAddr.String(), nil)
	ifs, err := net.Interfaces()
	require.NoError(t, err)

	cfg := testConfig()
	cfg.EnableGatewayLookup = true
	tr, err := newTracer(cfg, nil)
	require.NoError(t, err)
	require.NotNil(t, tr)
	t.Cleanup(tr.Stop)
	require.NotNil(t, tr.gwLookup)

	network.SubnetForHwAddrFunc = func(hwAddr net.HardwareAddr) (network.Subnet, error) {
		t.Logf("subnet lookup: %s", hwAddr)
		for _, i := range ifs {
			if hwAddr.String() == i.HardwareAddr.String() {
				return network.Subnet{Alias: fmt.Sprintf("subnet-%d", i.Index)}, nil
			}
		}

		return network.Subnet{Alias: "subnet"}, nil
	}

	require.NoError(t, tr.start(), "could not start tracer")

	initTracerState(t, tr)

	var clientIP string
	var clientPort int
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		clientIP, clientPort, _, err = testdns.SendDNSQueries(t, []string{"google.com"}, dnsAddr, "udp")
		assert.NoError(c, err)
	}, 6*time.Second, 100*time.Millisecond, "failed to send dns query")

	dnsClientAddr := &net.UDPAddr{IP: net.ParseIP(clientIP), Port: clientPort}
	dnsServerAddr := &net.UDPAddr{IP: dnsAddr, Port: 53}

	var conn *network.ConnectionStats
	require.Eventually(t, func() bool {
		var ok bool
		conn, ok = findConnection(dnsClientAddr, dnsServerAddr, getConnections(t, tr))
		return ok
	}, 3*time.Second, 100*time.Millisecond)

	require.NotNil(t, conn.Via, "connection is missing via: %s", conn)
	require.Equal(t, conn.Via.Subnet.Alias, fmt.Sprintf("subnet-%d", ifi.Index))
}

func (s *TracerSuite) TestGatewayLookupSubnetLookupError() {
	t := s.T()
	ctrl := gomock.NewController(t)
	m := NewMockcloudProvider(ctrl)
	oldCloud := network.Cloud
	defer func() {
		network.Cloud = oldCloud
	}()

	m.EXPECT().IsAWS().Return(true)
	network.Cloud = m

	destAddr := net.ParseIP("8.8.8.8")
	destDomain := "google.com"
	cfg := testConfig()
	cfg.EnableGatewayLookup = true
	// create the tracer without starting it
	tr, err := newTracer(cfg, nil)
	require.NoError(t, err)
	require.NotNil(t, tr)
	t.Cleanup(tr.Stop)
	require.NotNil(t, tr.gwLookup)

	ifi := ipRouteGet(t, "", destAddr.String(), nil)
	calls := 0
	network.SubnetForHwAddrFunc = func(hwAddr net.HardwareAddr) (network.Subnet, error) {
		if hwAddr.String() == ifi.HardwareAddr.String() {
			calls++
		}
		return network.Subnet{}, assert.AnError
	}

	require.NoError(t, tr.start(), "failed to start tracer")

	initTracerState(t, tr)

	var clientIP string
	var clientPort int
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		clientIP, clientPort, _, err = testdns.SendDNSQueries(t, []string{destDomain}, destAddr, "udp")
		assert.NoError(c, err)
	}, 6*time.Second, 100*time.Millisecond, "failed to send dns query")

	dnsClientAddr := &net.UDPAddr{IP: net.ParseIP(clientIP), Port: clientPort}
	dnsServerAddr := &net.UDPAddr{IP: destAddr, Port: 53}
	var c *network.ConnectionStats
	require.Eventually(t, func() bool {
		var ok bool
		c, ok = findConnection(dnsClientAddr, dnsServerAddr, getConnections(t, tr))
		return ok
	}, 3*time.Second, 100*time.Millisecond, "connection not found")
	require.Nil(t, c.Via)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		clientIP, clientPort, _, err = testdns.SendDNSQueries(t, []string{destDomain}, destAddr, "udp")
		assert.NoError(c, err)
	}, 6*time.Second, 100*time.Millisecond, "failed to send dns query")

	dnsClientAddr = &net.UDPAddr{IP: net.ParseIP(clientIP), Port: clientPort}
	require.Eventually(t, func() bool {
		var ok bool
		c, ok = findConnection(dnsClientAddr, dnsServerAddr, getConnections(t, tr))
		return ok
	}, 3*time.Second, 100*time.Millisecond, "connection not found")
	require.Nil(t, c.Via)

	require.Equal(t, 1, calls, "calls to subnetForHwAddrFunc are != 1 for hw addr %s", ifi.HardwareAddr)
}

func (s *TracerSuite) TestGatewayLookupCrossNamespace() {
	t := s.T()
	ctrl := gomock.NewController(t)
	m := NewMockcloudProvider(ctrl)
	oldCloud := network.Cloud
	defer func() {
		network.Cloud = oldCloud
	}()

	m.EXPECT().IsAWS().Return(true)
	network.Cloud = m

	cfg := testConfig()
	cfg.EnableGatewayLookup = true
	tr, err := newTracer(cfg, nil)
	require.NoError(t, err)
	require.NotNil(t, tr)
	t.Cleanup(tr.Stop)
	require.NotNil(t, tr.gwLookup)

	ns1 := netlinktestutil.AddNS(t)
	ns2 := netlinktestutil.AddNS(t)

	// setup two network namespaces
	t.Cleanup(func() {
		testutil.RunCommands(t, []string{
			"ip link del veth1",
			"ip link del veth3",
			"ip link del br0",
		}, true)
	})
	testutil.IptablesSave(t)
	cmds := []string{
		"ip link add br0 type bridge",
		"ip addr add 2.2.2.1/24 broadcast 2.2.2.255 dev br0",
		"ip link add veth1 type veth peer name veth2",
		"ip link set veth1 master br0",
		fmt.Sprintf("ip link set veth2 netns %s", ns1),
		fmt.Sprintf("ip -n %s addr add 2.2.2.2/24 broadcast 2.2.2.255 dev veth2", ns1),
		"ip link add veth3 type veth peer name veth4",
		"ip link set veth3 master br0",
		fmt.Sprintf("ip link set veth4 netns %s", ns2),
		fmt.Sprintf("ip -n %s addr add 2.2.2.3/24 broadcast 2.2.2.255 dev veth4", ns2),
		"ip link set br0 up",
		"ip link set veth1 up",
		fmt.Sprintf("ip -n %s link set veth2 up", ns1),
		"ip link set veth3 up",
		fmt.Sprintf("ip -n %s link set veth4 up", ns2),
		fmt.Sprintf("ip -n %s r add default via 2.2.2.1", ns1),
		fmt.Sprintf("ip -n %s r add default via 2.2.2.1", ns2),
		"iptables -I POSTROUTING 1 -t nat -s 2.2.2.0/24 ! -d 2.2.2.0/24 -j MASQUERADE",
		"iptables -I FORWARD -i br0 -j ACCEPT",
		"iptables -I FORWARD -o br0 -j ACCEPT",
		"sysctl -w net.ipv4.ip_forward=1",
	}
	testutil.RunCommands(t, cmds, false)

	ifs, err := net.Interfaces()
	require.NoError(t, err)
	network.SubnetForHwAddrFunc = func(hwAddr net.HardwareAddr) (network.Subnet, error) {
		for _, i := range ifs {
			if hwAddr.String() == i.HardwareAddr.String() {
				return network.Subnet{Alias: fmt.Sprintf("subnet-%s", i.Name)}, nil
			}
		}

		return network.Subnet{Alias: "subnet"}, nil
	}

	require.NoError(t, tr.start(), "could not start tracer")

	test1Ns, err := vnetns.GetFromName(ns1)
	require.NoError(t, err)
	defer test1Ns.Close()

	// run tcp server in test1 net namespace
	var server *tracertestutil.TCPServer
	err = kernel.WithNS(test1Ns, func() error {
		server = tracertestutil.NewTCPServerOnAddress("2.2.2.2:0", func(_ net.Conn) {})
		return server.Run()
	})
	require.NoError(t, err)
	t.Cleanup(server.Shutdown)

	var conn *network.ConnectionStats
	t.Run("client in root namespace", func(t *testing.T) {
		c, err := net.DialTimeout("tcp", server.Address(), 2*time.Second)
		require.NoError(t, err)

		// write some data
		_, err = c.Write([]byte("foo"))
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			var ok bool
			conns := getConnections(t, tr)
			t.Log(conns)
			conn, ok = findConnection(c.LocalAddr(), c.RemoteAddr(), conns)
			return ok && conn.Direction == network.OUTGOING
		}, 3*time.Second, 100*time.Millisecond)

		// conn.Via should be nil, since traffic is local
		require.Nil(t, conn.Via)
	})

	t.Run("client in other namespace", func(t *testing.T) {
		skipOnEbpflessNotSupported(t, cfg)
		// try connecting to server in test1 namespace
		test2Ns, err := vnetns.GetFromName(ns2)
		require.NoError(t, err)
		defer test2Ns.Close()

		var c net.Conn
		err = kernel.WithNS(test2Ns, func() error {
			var err error
			c, err = net.DialTimeout("tcp", server.Address(), 2*time.Second)
			return err
		})
		require.NoError(t, err)
		defer c.Close()

		// write some data
		_, err = c.Write([]byte("foo"))
		require.NoError(t, err)

		var conn *network.ConnectionStats
		require.Eventually(t, func() bool {
			var ok bool
			conns := getConnections(t, tr)
			t.Log(conns)
			conn, ok = findConnection(c.LocalAddr(), c.RemoteAddr(), conns)
			return ok && conn.Direction == network.OUTGOING
		}, 3*time.Second, 100*time.Millisecond)

		// traffic is local, so Via field should not be set
		require.Nil(t, conn.Via)

		// try connecting to something outside
		dnsAddr := net.ParseIP("8.8.8.8")
		var dnsClientAddr, dnsServerAddr *net.UDPAddr
		var clientIP string
		var clientPort int
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			kernel.WithNS(test2Ns, func() error {
				clientIP, clientPort, _, err = testdns.SendDNSQueries(t, []string{"google.com"}, dnsAddr, "udp")
				return nil
			})
			assert.NoError(c, err)
		}, 6*time.Second, 100*time.Millisecond, "failed to send dns query")

		dnsClientAddr = &net.UDPAddr{IP: net.ParseIP(clientIP), Port: clientPort}
		dnsServerAddr = &net.UDPAddr{IP: dnsAddr, Port: 53}

		iif := ipRouteGet(t, "", dnsClientAddr.IP.String(), nil)
		ifi := ipRouteGet(t, dnsClientAddr.IP.String(), dnsAddr.String(), iif)

		require.Eventually(t, func() bool {
			var ok bool
			conn, ok = findConnection(dnsClientAddr, dnsServerAddr, getConnections(t, tr))
			return ok && conn.Direction == network.OUTGOING
		}, 3*time.Second, 100*time.Millisecond)

		require.NotNil(t, conn.Via)
		require.Equal(t, fmt.Sprintf("subnet-%s", ifi.Name), conn.Via.Subnet.Alias)

	})
}

func (s *TracerSuite) TestConnectionAssured() {
	t := s.T()
	cfg := testConfig()
	skipOnEbpflessNotSupported(t, cfg)

	tr := setupTracer(t, cfg)
	server := &UDPServer{
		network: "udp4",
		onMessage: func([]byte, int) []byte {
			return genPayload(serverMessageSize)
		},
	}

	err := server.Run(clientMessageSize)
	require.NoError(t, err)
	t.Cleanup(server.Shutdown)

	c, err := net.DialTimeout("udp", server.address, time.Second)
	require.NoError(t, err)
	defer c.Close()

	// do two exchanges to make the connection "assured"
	for i := 0; i < 2; i++ {
		_, err = c.Write(genPayload(clientMessageSize))
		require.NoError(t, err)

		buf := make([]byte, serverMessageSize)
		_, err = c.Read(buf)
		require.NoError(t, err)
	}

	var conn *network.ConnectionStats
	require.Eventually(t, func() bool {
		conns := getConnections(t, tr)
		var ok bool
		conn, ok = findConnection(c.LocalAddr(), c.RemoteAddr(), conns)
		return ok && conn.Monotonic.SentBytes > 0 && conn.Monotonic.RecvBytes > 0
	}, 3*time.Second, 100*time.Millisecond, "could not find udp connection")

	// verify the connection is marked as assured
	require.True(t, conn.IsAssured)
}

func (s *TracerSuite) TestConnectionNotAssured() {
	t := s.T()
	cfg := testConfig()
	tr := setupTracer(t, cfg)

	server := &UDPServer{
		network: "udp4",
		onMessage: func([]byte, int) []byte {
			return nil
		},
	}

	err := server.Run(clientMessageSize)
	require.NoError(t, err)
	t.Cleanup(server.Shutdown)

	c, err := net.DialTimeout("udp", server.address, time.Second)
	require.NoError(t, err)
	defer c.Close()

	_, err = c.Write(genPayload(clientMessageSize))
	require.NoError(t, err)

	var conn *network.ConnectionStats
	require.Eventually(t, func() bool {
		conns := getConnections(t, tr)
		var ok bool
		conn, ok = findConnection(c.LocalAddr(), c.RemoteAddr(), conns)
		return ok && conn.Monotonic.SentBytes > 0 && conn.Monotonic.RecvBytes == 0
	}, 3*time.Second, 100*time.Millisecond, "could not find udp connection")

	// verify the connection is marked as not assured
	require.False(t, conn.IsAssured)
}

func (s *TracerSuite) TestUDPConnExpiryTimeout() {
	t := s.T()
	streamTimeout, err := sysctl.NewInt("/proc", "net/netfilter/nf_conntrack_udp_timeout_stream", 0).Get()
	require.NoError(t, err)
	timeout, err := sysctl.NewInt("/proc", "net/netfilter/nf_conntrack_udp_timeout", 0).Get()
	require.NoError(t, err)

	tr := setupTracer(t, testConfig())
	require.Equal(t, uint64(time.Duration(timeout)*time.Second), tr.udpConnTimeout(false))
	require.Equal(t, uint64(time.Duration(streamTimeout)*time.Second), tr.udpConnTimeout(true))
}

func (s *TracerSuite) TestDNATIntraHostIntegration() {
	t := s.T()
	cfg := testConfig()
	skipEbpflessTodo(t, cfg)
	netlinktestutil.SetupDNAT(t)

	tr := setupTracer(t, cfg)

	var serverAddr struct {
		local, remote net.Addr
	}
	server := tracertestutil.NewTCPServerOnAddress("1.1.1.1:0", func(c net.Conn) {
		serverAddr.local = c.LocalAddr()
		serverAddr.remote = c.RemoteAddr()
		for {
			bs := make([]byte, 4)
			_, err := c.Read(bs)
			if err == io.EOF {
				return
			}
			require.NoError(t, err, "error reading in server")

			_, err = c.Write([]byte("pong"))
			require.NoError(t, err, "error writing back in server")
		}
	})
	t.Cleanup(server.Shutdown)
	require.NoError(t, server.Run())

	_, port, err := net.SplitHostPort(server.Address())
	require.NoError(t, err)

	var conn net.Conn
	conn, err = net.Dial("tcp", "2.2.2.2:"+port)
	require.NoError(t, err, "error connecting to client")
	t.Cleanup(func() {
		conn.Close()
	})

	var incoming, outgoing *network.ConnectionStats
	require.Eventually(t, func() bool {
		_, err = conn.Write([]byte("ping"))
		if !assert.NoError(t, err, "error writing in client") {
			return false
		}

		bs := make([]byte, 4)
		_, err = conn.Read(bs)
		if !assert.NoError(t, err) {
			return false
		}

		conns := getConnections(t, tr)
		t.Log(conns)

		outgoing, _ = findConnection(conn.LocalAddr(), conn.RemoteAddr(), conns)
		incoming, _ = findConnection(serverAddr.local, serverAddr.remote, conns)

		t.Logf("incoming: %+v, outgoing: %+v", incoming, outgoing)

		return outgoing != nil && incoming != nil && outgoing.IPTranslation != nil
	}, 3*time.Second, 100*time.Millisecond, "failed to get both incoming and outgoing connection")

	assert.True(t, outgoing.IntraHost, "did not find outgoing connection classified as local: %v", outgoing)
	assert.True(t, incoming.IntraHost, "did not find incoming connection classified as local: %v", incoming)
}

func (s *TracerSuite) TestSelfConnect() {
	t := s.T()
	// Enable BPF-based system probe
	cfg := testConfig()
	cfg.TCPConnTimeout = 3 * time.Second
	tr := setupTracer(t, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	t.Cleanup(cancel)

	cmd := exec.CommandContext(ctx, "testdata/fork.py")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.WaitDelay = 10 * time.Second
	stdOutReader, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		cancel()
		if err := cmd.Wait(); err != nil {
			status := cmd.ProcessState.Sys().(syscall.WaitStatus)
			assert.Equal(t, syscall.SIGKILL, status.Signal(), "fork.py output: %s", stderr.String())
		}
	})

	portStr, err := bufio.NewReader(stdOutReader).ReadString('\n')
	require.NoError(t, err, "error reading port from fork.py")

	port, err := strconv.ParseUint(strings.TrimSpace(portStr), 10, 16)
	require.NoError(t, err, "could not convert %s to integer port", portStr)

	t.Logf("port is %d", port)

	require.Eventually(t, func() bool {
		conns := network.FilterConnections(getConnections(t, tr), func(cs network.ConnectionStats) bool {
			return cs.SPort == uint16(port) && cs.DPort == uint16(port) && cs.Source.IsLoopback() && cs.Dest.IsLoopback()
		})

		t.Logf("connections: %v", conns)
		return len(conns) == 2
	}, 5*time.Second, 100*time.Millisecond, "could not find expected number of tcp connections, expected: 2")
}

// sets up two udp sockets talking to each other locally.
// returns (listener, dialer)
func setupUdpSockets(t *testing.T, udpnet, ip string) (*net.UDPConn, *net.UDPConn) {
	serverAddr := fmt.Sprintf("%s:%d", ip, 0)

	laddr, err := net.ResolveUDPAddr(udpnet, serverAddr)
	require.NoError(t, err)

	var ln, c *net.UDPConn = nil, nil
	t.Cleanup(func() {
		if ln != nil {
			ln.Close()
		}
		if c != nil {
			c.Close()
		}
	})

	ln, err = net.ListenUDP(udpnet, laddr)
	require.NoError(t, err)

	saddr := ln.LocalAddr().String()

	raddr, err := net.ResolveUDPAddr(udpnet, saddr)
	require.NoError(t, err)

	c, err = net.DialUDP(udpnet, laddr, raddr)
	require.NoError(t, err)

	return ln, c
}

func (s *TracerSuite) TestUDPPeekCount() {
	t := s.T()
	t.Run("v4", func(t *testing.T) {
		testUDPPeekCount(t, "udp4", "127.0.0.1")
	})
	t.Run("v6", func(t *testing.T) {
		if !testConfig().CollectUDPv6Conns {
			t.Skip("UDPv6 disabled")
		}
		testUDPPeekCount(t, "udp6", "[::1]")
	})
}
func testUDPPeekCount(t *testing.T, udpnet, ip string) {
	config := testConfig()
	tr := setupTracer(t, config)

	ln, c := setupUdpSockets(t, udpnet, ip)

	msg := []byte("asdf")
	_, err := c.Write(msg)
	require.NoError(t, err)

	rawConn, err := ln.SyscallConn()
	require.NoError(t, err)
	err = rawConn.Control(func(fd uintptr) {
		buf := make([]byte, 1024)
		var n int
		var err error
		done := make(chan struct{})

		recv := func(flags int) {
			for {
				n, _, err = syscall.Recvfrom(int(fd), buf, flags)
				if err == syscall.EINTR || err == syscall.EAGAIN {
					continue
				}
				break
			}
		}
		go func() {
			defer close(done)
			recv(syscall.MSG_PEEK)
			if n == 0 || err != nil {
				return
			}
			recv(0)
		}()

		select {
		case <-done:
			require.NoError(t, err)
			require.NotZero(t, n)
		case <-time.After(5 * time.Second):
			require.Fail(t, "receive timed out")
		}
	})
	require.NoError(t, err)

	var incoming *network.ConnectionStats
	var outgoing *network.ConnectionStats
	require.Eventuallyf(t, func() bool {
		conns := getConnections(t, tr)
		if outgoing == nil {
			outgoing, _ = findConnection(c.LocalAddr(), c.RemoteAddr(), conns)
		}
		if incoming == nil {
			incoming, _ = findConnection(c.RemoteAddr(), c.LocalAddr(), conns)
		}

		return outgoing != nil && incoming != nil
	}, 3*time.Second, 100*time.Millisecond, "couldn't find incoming and outgoing connections matching")

	m := outgoing.Monotonic
	require.Equal(t, len(msg), int(m.SentBytes))
	require.Equal(t, 0, int(m.RecvBytes))
	require.Equal(t, 1, int(m.SentPackets))
	require.Equal(t, 0, int(m.RecvPackets))
	require.True(t, outgoing.IntraHost)

	// make sure the inverse values are seen for the other message
	m = incoming.Monotonic
	require.Equal(t, 0, int(m.SentBytes))
	require.Equal(t, len(msg), int(m.RecvBytes))
	require.Equal(t, 0, int(m.SentPackets))
	require.Equal(t, 1, int(m.RecvPackets))
	require.True(t, incoming.IntraHost)
}

func (s *TracerSuite) TestUDPPacketSumming() {
	t := s.T()
	t.Run("v4", func(t *testing.T) {
		testUDPPacketSumming(t, "udp4", "127.0.0.1")
	})
	t.Run("v6", func(t *testing.T) {
		if !testConfig().CollectUDPv6Conns {
			t.Skip("UDPv6 disabled")
		}
		testUDPPacketSumming(t, "udp6", "[::1]")
	})
}
func testUDPPacketSumming(t *testing.T, udpnet, ip string) {
	config := testConfig()
	tr := setupTracer(t, config)

	ln, c := setupUdpSockets(t, udpnet, ip)

	msg := []byte("asdf")
	// send UDP packets of increasing length
	for i := range msg {
		_, err := c.Write(msg[:i+1])
		require.NoError(t, err)
	}
	expectedBytes := 1 + 2 + 3 + 4

	buf := make([]byte, 256)
	recvBytes := 0
	for range msg {
		n, _, err := ln.ReadFrom(buf)
		require.NoError(t, err)
		recvBytes += n
	}
	// sanity check: did userspace get all four expected packets?
	require.Equal(t, recvBytes, expectedBytes)

	var incoming *network.ConnectionStats
	var outgoing *network.ConnectionStats
	require.Eventuallyf(t, func() bool {
		conns := getConnections(t, tr)
		if outgoing == nil {
			outgoing, _ = findConnection(c.LocalAddr(), c.RemoteAddr(), conns)
		}
		if incoming == nil {
			incoming, _ = findConnection(c.RemoteAddr(), c.LocalAddr(), conns)
		}

		return outgoing != nil && incoming != nil
	}, 3*time.Second, 100*time.Millisecond, "couldn't find incoming and outgoing connections matching")

	m := outgoing.Monotonic
	require.Equal(t, expectedBytes, int(m.SentBytes))
	require.Equal(t, 0, int(m.RecvBytes))
	require.Equal(t, int(len(msg)), int(m.SentPackets))
	require.Equal(t, 0, int(m.RecvPackets))
	require.True(t, outgoing.IntraHost)

	// make sure the inverse values are seen for the other message
	m = incoming.Monotonic
	require.Equal(t, 0, int(m.SentBytes))
	require.Equal(t, expectedBytes, int(m.RecvBytes))
	require.Equal(t, 0, int(m.SentPackets))
	require.Equal(t, int(len(msg)), int(m.RecvPackets))
	require.True(t, incoming.IntraHost)
}

func (s *TracerSuite) TestUDPPythonReusePort() {
	t := s.T()
	cfg := testConfig()
	if isPrebuilt(cfg) && kv < kv470 {
		t.Skip("reuseport not supported on prebuilt")
	}

	tr := setupTracer(t, cfg)

	var out string
	var err error
	for i := 0; i < 5; i++ {
		err = func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			out, err = testutil.RunCommandWithContext(ctx, "testdata/reuseport.py")
			if err != nil {
				t.Logf("error running reuseport.py: %s", err)
			}

			return err
		}()

		if err == nil {
			break
		}
	}

	require.NoError(t, err, "error running reuseport.py")

	port, err := strconv.ParseUint(strings.TrimSpace(strings.Split(out, "\n")[0]), 10, 16)
	require.NoError(t, err, "could not convert %s to integer port", out)

	t.Logf("port is %d", port)

	conns := map[network.ConnectionTuple]network.ConnectionStats{}
	require.Eventually(t, func() bool {
		_conns := network.FilterConnections(getConnections(t, tr), func(cs network.ConnectionStats) bool {
			return cs.Type == network.UDP &&
				cs.Source.IsLoopback() &&
				cs.Dest.IsLoopback() &&
				(cs.DPort == uint16(port) || cs.SPort == uint16(port))
		})

		for _, c := range _conns {
			conns[c.ConnectionTuple] = c
		}

		t.Log(conns)

		return len(conns) == 4
	}, 3*time.Second, 100*time.Millisecond, "could not find expected number of udp connections, expected: 4")

	var incoming, outgoing []network.ConnectionStats
	for _, c := range conns {
		if c.SPort == uint16(port) {
			incoming = append(incoming, c)
		} else if c.DPort == uint16(port) {
			outgoing = append(outgoing, c)
		}
	}

	serverBytes, clientBytes := 3, 6
	if assert.Len(t, incoming, 2, "unable to find incoming connections") {
		for _, c := range incoming {
			assert.Equal(t, network.INCOMING, c.Direction, "incoming direction")

			// make sure the inverse values are seen for the other message
			assert.Equal(t, serverBytes, int(c.Monotonic.SentBytes), "incoming sent")
			assert.Equal(t, clientBytes, int(c.Monotonic.RecvBytes), "incoming recv")
			assert.True(t, c.IntraHost, "incoming intrahost")
		}
	}

	if assert.Len(t, outgoing, 2, "unable to find outgoing connections") {
		for _, c := range outgoing {
			assert.Equal(t, network.OUTGOING, c.Direction, "outgoing direction")

			assert.Equal(t, clientBytes, int(c.Monotonic.SentBytes), "outgoing sent")
			assert.Equal(t, serverBytes, int(c.Monotonic.RecvBytes), "outgoing recv")
			assert.True(t, c.IntraHost, "outgoing intrahost")
		}
	}
}

func (s *TracerSuite) TestUDPReusePort() {
	t := s.T()
	t.Run("v4", func(t *testing.T) {
		testUDPReusePort(t, "udp4", "127.0.0.1")
	})
	t.Run("v6", func(t *testing.T) {
		if !testConfig().CollectUDPv6Conns {
			t.Skip("UDPv6 disabled")
		}
		testUDPReusePort(t, "udp6", "[::1]")
	})
}

func testUDPReusePort(t *testing.T, udpnet string, ip string) {
	cfg := testConfig()
	if isPrebuilt(cfg) && kv < kv470 {
		t.Skip("reuseport not supported on prebuilt")
	}

	tr := setupTracer(t, cfg)

	createReuseServer := func(port int) *UDPServer {
		return &UDPServer{
			network: udpnet,
			lc: &net.ListenConfig{
				Control: func(_, _ string, c syscall.RawConn) error {
					var opErr error
					err := c.Control(func(fd uintptr) {
						opErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
					})
					if err != nil {
						return err
					}
					return opErr
				},
			},
			address: fmt.Sprintf("%s:%d", ip, port),
			onMessage: func(_ []byte, _ int) []byte {
				return genPayload(serverMessageSize)
			},
		}
	}

	s1 := createReuseServer(0)
	err := s1.Run(clientMessageSize)
	assignedPort := s1.ln.LocalAddr().(*net.UDPAddr).Port
	require.NoError(t, err)
	t.Cleanup(s1.Shutdown)

	s2 := createReuseServer(assignedPort)
	err = s2.Run(clientMessageSize)
	require.NoError(t, err)
	t.Cleanup(s2.Shutdown)

	// Connect to server
	c, err := net.DialTimeout(udpnet, s1.address, 50*time.Millisecond)
	require.NoError(t, err)
	defer c.Close()

	// Write clientMessageSize to server, and read response
	_, err = c.Write(genPayload(clientMessageSize))
	require.NoError(t, err)

	_, err = c.Read(make([]byte, serverMessageSize))
	require.NoError(t, err)

	// Iterate through active connections until we find connection created above, and confirm send + recv counts
	t.Logf("port: %d", assignedPort)
	connections := getConnections(t, tr)
	for _, c := range connections.Conns {
		t.Log(c)
	}

	incoming, ok := findConnection(c.RemoteAddr(), c.LocalAddr(), connections)
	if assert.True(t, ok, "unable to find incoming connection") {
		assert.Equal(t, network.INCOMING, incoming.Direction)

		// make sure the inverse values are seen for the other message
		assert.Equal(t, serverMessageSize, int(incoming.Monotonic.SentBytes), "incoming sent")
		assert.Equal(t, clientMessageSize, int(incoming.Monotonic.RecvBytes), "incoming recv")
		assert.True(t, incoming.IntraHost, "incoming intrahost")
	}

	outgoing, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	if assert.True(t, ok, "unable to find outgoing connection") {
		assert.Equal(t, network.OUTGOING, outgoing.Direction)

		assert.Equal(t, clientMessageSize, int(outgoing.Monotonic.SentBytes), "outgoing sent")
		assert.Equal(t, serverMessageSize, int(outgoing.Monotonic.RecvBytes), "outgoing recv")
		assert.True(t, outgoing.IntraHost, "outgoing intrahost")
	}
}

func (s *TracerSuite) TestDNSStatsWithNAT() {
	t := s.T()
	cfg := testConfig()
	skipEbpflessTodo(t, cfg)
	testutil.IptablesSave(t)
	// Setup a NAT rule to translate 2.2.2.2 to 8.8.8.8 and issue a DNS request to 2.2.2.2
	cmds := []string{"iptables -t nat -A OUTPUT -d 2.2.2.2 -j DNAT --to-destination 8.8.8.8"}
	testutil.RunCommands(t, cmds, false)

	cfg.CollectDNSStats = true
	cfg.DNSTimeout = 1 * time.Second
	tr := setupTracer(t, cfg)

	t.Logf("requesting golang.com@2.2.2.2 with conntrack type: %T", tr.conntracker)
	testDNSStats(t, tr, "golang.org", 1, 0, 0, "2.2.2.2")
}

func iptablesWrapper(t *testing.T, f func()) {
	iptables, err := exec.LookPath("iptables")
	require.NoError(t, err)

	// Init iptables rule to simulate packet loss
	rule := "INPUT --source 127.0.0.1 -j DROP"
	create := strings.Fields(fmt.Sprintf("-I %s", rule))

	state := testutil.IptablesSave(t)
	defer testutil.IptablesRestore(t, state)
	createCmd := exec.Command(iptables, create...)
	err = createCmd.Run()
	require.NoError(t, err)

	f()
}

func ipRouteGet(t *testing.T, from, dest string, iif *net.Interface) *net.Interface {
	ipRouteGetOut := regexp.MustCompile(`dev\s+([^\s/]+)`)

	args := []string{"route", "get"}
	if len(from) > 0 {
		args = append(args, "from", from)
	}
	args = append(args, dest)
	if iif != nil {
		args = append(args, "iif", iif.Name)
	}
	cmd := exec.Command("ip", args...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "ip command returned error, output: %s", out)
	t.Log(strings.Join(cmd.Args, " "))
	t.Log(string(out))

	matches := ipRouteGetOut.FindSubmatch(out)
	require.Len(t, matches, 2, string(out))
	dev := string(matches[1])
	ifi, err := net.InterfaceByName(dev)
	require.NoError(t, err)
	return ifi
}

type SyscallConn interface {
	net.Conn
	SyscallConn() (syscall.RawConn, error)
}

func (s *TracerSuite) TestSendfileRegression() {
	t := s.T()
	// Start tracer
	cfg := testConfig()
	tr := setupTracer(t, cfg)

	// Create temporary file
	tmpdir := t.TempDir()
	tmpfilePath := filepath.Join(tmpdir, "sendfile_source")
	tmpfile, err := os.Create(tmpfilePath)
	require.NoError(t, err)

	n, err := tmpfile.Write(genPayload(clientMessageSize))
	require.NoError(t, err)
	require.Equal(t, clientMessageSize, n)

	// Grab file size
	stat, err := tmpfile.Stat()
	require.NoError(t, err)
	fsize := int(stat.Size())

	testSendfileServer := func(t *testing.T, c SyscallConn, connType network.ConnectionType, family network.ConnectionFamily, rcvdFunc func() int64) {
		_, err = tmpfile.Seek(0, 0)
		require.NoError(t, err)

		// Send file contents via SENDFILE(2)
		n, err = sendFile(t, c, tmpfile, nil, fsize)
		require.NoError(t, err)
		require.Equal(t, fsize, n)

		// Verify that our server received the contents of the file
		c.Close()
		require.Eventually(t, func() bool {
			return int64(clientMessageSize) == rcvdFunc()
		}, 3*time.Second, 100*time.Millisecond, "TCP server didn't receive data")

		t.Logf("looking for connections %+v <-> %+v", c.LocalAddr(), c.RemoteAddr())
		var outConn, inConn *network.ConnectionStats
		assert.Eventually(t, func() bool {
			conns := getConnections(t, tr)
			t.Log(conns)
			if outConn == nil {
				outConn = network.FirstConnection(conns, network.ByType(connType), network.ByFamily(family), network.ByTuple(c.LocalAddr(), c.RemoteAddr()))
			}
			if inConn == nil {
				inConn = network.FirstConnection(conns, network.ByType(connType), network.ByFamily(family), network.ByTuple(c.RemoteAddr(), c.LocalAddr()))
			}
			return outConn != nil && inConn != nil
		}, 3*time.Second, 100*time.Millisecond, "couldn't find connections used by sendfile(2)")

		if assert.NotNil(t, outConn, "couldn't find outgoing connection used by sendfile(2)") {
			assert.Equalf(t, int64(clientMessageSize), int64(outConn.Monotonic.SentBytes), "sendfile sent bytes wasn't properly traced")
			if connType == network.UDP {
				assert.Equalf(t, int64(1), int64(outConn.Monotonic.SentPackets), "sendfile UDP should send exactly 1 packet")
				assert.Equalf(t, int64(0), int64(outConn.Monotonic.RecvPackets), "sendfile outConn shouldn't have any RecvPackets")
			}
		}
		if assert.NotNil(t, inConn, "couldn't find incoming connection used by sendfile(2)") {
			assert.Equalf(t, int64(clientMessageSize), int64(inConn.Monotonic.RecvBytes), "sendfile recv bytes wasn't properly traced")
			if connType == network.UDP {
				assert.Equalf(t, int64(1), int64(inConn.Monotonic.RecvPackets), "sendfile UDP should recv exactly 1 packet")
				assert.Equalf(t, int64(0), int64(inConn.Monotonic.SentPackets), "sendfile inConn shouldn't have any SentPackets")
			}
		}
	}

	for _, family := range []network.ConnectionFamily{network.AFINET, network.AFINET6} {
		t.Run(family.String(), func(t *testing.T) {
			t.Run("TCP", func(t *testing.T) {
				// Start TCP server
				var rcvd int64
				server := tracertestutil.NewTCPServerOnAddress("", func(c net.Conn) {
					rcvd, _ = io.Copy(io.Discard, c)
					c.Close()
				})
				server.Network = "tcp" + strings.TrimPrefix(family.String(), "v")
				t.Cleanup(server.Shutdown)
				require.NoError(t, server.Run())

				// Connect to TCP server
				c, err := net.DialTimeout("tcp", server.Address(), time.Second)
				require.NoError(t, err)

				testSendfileServer(t, c.(*net.TCPConn), network.TCP, family, func() int64 { return rcvd })
			})
			t.Run("UDP", func(t *testing.T) {
				if family == network.AFINET6 && !cfg.CollectUDPv6Conns {
					t.Skip("UDPv6 disabled")
				}
				if isPrebuilt(cfg) && kv < kv470 {
					t.Skip("UDP will fail with prebuilt tracer")
				}

				// Start UDP server
				var rcvd int64
				server := &UDPServer{
					network: "udp" + strings.TrimPrefix(family.String(), "v"),
					onMessage: func(_ []byte, n int) []byte {
						rcvd = rcvd + int64(n)
						return nil
					},
				}
				t.Cleanup(server.Shutdown)
				require.NoError(t, server.Run(1024))

				// Connect to UDP server
				c, err := net.DialTimeout(server.network, server.address, time.Second)
				require.NoError(t, err)

				testSendfileServer(t, c.(*net.UDPConn), network.UDP, family, func() int64 { return rcvd })
			})
		})
	}

}

func httpSupported() bool {
	if ebpftest.GetBuildMode() == ebpftest.Fentry {
		return false
	}
	return kv >= usmconfig.MinimumKernelVersion
}

func isPrebuilt(cfg *config.Config) bool {
	if cfg.EnableRuntimeCompiler || cfg.EnableCORE {
		return false
	}
	return true
}

func (s *TracerSuite) TestSendfileError() {
	t := s.T()
	tr := setupTracer(t, testConfig())

	tmpfile, err := os.CreateTemp("", "sendfile_source")
	require.NoError(t, err)
	t.Cleanup(func() { os.Remove(tmpfile.Name()) })

	n, err := tmpfile.Write(genPayload(clientMessageSize))
	require.NoError(t, err)
	require.Equal(t, clientMessageSize, n)
	_, err = tmpfile.Seek(0, 0)
	require.NoError(t, err)

	server := tracertestutil.NewTCPServer(func(c net.Conn) {
		_, _ = io.Copy(io.Discard, c)
		c.Close()
	})
	require.NoError(t, server.Run())
	t.Cleanup(server.Shutdown)

	c, err := net.DialTimeout("tcp", server.Address(), time.Second)
	require.NoError(t, err)

	// Send file contents via SENDFILE(2)
	offset := int64(math.MaxInt64 - 1)
	_, err = sendFile(t, c.(*net.TCPConn), tmpfile, &offset, 10)
	require.Error(t, err)

	c.Close()

	var conn *network.ConnectionStats
	require.Eventually(t, func() bool {
		conns := getConnections(t, tr)
		var ok bool
		conn, ok = findConnection(c.LocalAddr(), c.RemoteAddr(), conns)
		return ok
	}, 3*time.Second, 100*time.Millisecond, "couldn't find connection used by sendfile(2)")

	assert.Equalf(t, int64(0), int64(conn.Monotonic.SentBytes), "sendfile data wasn't properly traced")
}

func sendFile(t *testing.T, c SyscallConn, f *os.File, offset *int64, count int) (int, error) {
	// Send payload using SENDFILE(2) syscall
	rawConn, err := c.SyscallConn()
	require.NoError(t, err)
	var n int
	var serr error
	err = rawConn.Control(func(fd uintptr) {
		n, serr = syscall.Sendfile(int(fd), int(f.Fd()), offset, count)
	})
	if err != nil {
		return 0, err
	}
	return n, serr
}

func (s *TracerSuite) TestShortWrite() {
	t := s.T()
	tr := setupTracer(t, testConfig())

	read := make(chan struct{})
	server := tracertestutil.NewTCPServer(func(c net.Conn) {
		// set recv buffer to 0 and don't read
		// to fill up tcp window
		err := c.(*net.TCPConn).SetReadBuffer(0)
		require.NoError(t, err)
		<-read
		c.Close()
	})
	require.NoError(t, server.Run())
	t.Cleanup(func() {
		close(read)
		server.Shutdown()
	})

	sk, err := unix.Socket(syscall.AF_INET, syscall.SOCK_STREAM|syscall.SOCK_NONBLOCK, 0)
	require.NoError(t, err)
	defer syscall.Close(sk)

	err = unix.SetsockoptInt(sk, syscall.SOL_SOCKET, syscall.SO_SNDBUF, 5000)
	require.NoError(t, err)

	sndBufSize, err := unix.GetsockoptInt(sk, syscall.SOL_SOCKET, syscall.SO_SNDBUF)
	require.NoError(t, err)
	require.GreaterOrEqual(t, sndBufSize, 5000)

	var sa unix.SockaddrInet4
	host, portStr, err := net.SplitHostPort(server.Address())
	require.NoError(t, err)
	copy(sa.Addr[:], net.ParseIP(host).To4())
	port, err := strconv.ParseInt(portStr, 10, 32)
	require.NoError(t, err)
	sa.Port = int(port)

	err = unix.Connect(sk, &sa)
	if syscall.EINPROGRESS != err {
		require.NoError(t, err)
	}

	var wfd unix.FdSet
	wfd.Zero()
	wfd.Set(sk)
	tv := unix.NsecToTimeval(int64((5 * time.Second).Nanoseconds()))
	nfds, err := unix.Select(sk+1, nil, &wfd, nil, &tv)
	require.NoError(t, err)
	require.Equal(t, 1, nfds)

	var written int
	done := false
	var sent uint64
	toSend := sndBufSize / 2
	for i := 0; i < 100; i++ {
		written, err = unix.Write(sk, genPayload(toSend))
		require.NoError(t, err)
		require.Greater(t, written, 0)
		sent += uint64(written)
		t.Logf("sent: %v", sent)
		if written < toSend {
			done = true
			break
		}
	}

	require.True(t, done)

	f := os.NewFile(uintptr(sk), "")
	defer f.Close()
	c, err := net.FileConn(f)
	require.NoError(t, err)

	var conn *network.ConnectionStats
	require.Eventually(t, func() bool {
		conns := getConnections(t, tr)
		var ok bool
		conn, ok = findConnection(c.LocalAddr(), c.RemoteAddr(), conns)
		return ok
	}, 3*time.Second, 100*time.Millisecond, "couldn't find connection used by short write")

	assert.Equal(t, sent, conn.Monotonic.SentBytes)
}

func (s *TracerSuite) TestKprobeAttachWithKprobeEvents() {
	t := s.T()
	cfg := config.New()
	skipOnEbpflessNotSupported(t, cfg)
	cfg.AttachKprobesWithKprobeEventsABI = true

	tr := setupTracer(t, cfg)

	if tr.ebpfTracer.Type() == connection.TracerTypeFentry {
		t.Skip("skipped on Fargate")
	}

	cmd := []string{"curl", "-k", "-o/dev/null", "example.com"}
	exec.Command(cmd[0], cmd[1:]...).Run()

	stats := ebpftelemetry.GetProbeStats()
	require.NotNil(t, stats)

	pTCPSendmsg, ok := stats["p_tcp_sendmsg_hits"]
	require.True(t, ok)
	fmt.Printf("p_tcp_sendmsg_hits = %d\n", pTCPSendmsg)

	assert.Greater(t, pTCPSendmsg, uint64(0))
}

func (s *TracerSuite) TestBlockingReadCounts() {
	t := s.T()
	tr := setupTracer(t, testConfig())
	ch := make(chan struct{})
	server := tracertestutil.NewTCPServer(func(c net.Conn) {
		_, err := c.Write([]byte("foo"))
		require.NoError(t, err, "error writing to client")
		time.Sleep(time.Second)
		_, err = c.Write([]byte("foo"))
		require.NoError(t, err, "error writing to client")
		<-ch
	})

	require.NoError(t, server.Run())
	t.Cleanup(server.Shutdown)
	t.Cleanup(func() { close(ch) })

	c, err := net.DialTimeout("tcp", server.Address(), 5*time.Second)
	require.NoError(t, err)
	defer c.Close()

	rawConn, err := c.(syscall.Conn).SyscallConn()
	require.NoError(t, err, "error getting raw conn")

	// set the socket to blocking as the MSG_WAITALL
	// option used later on for reads only works for
	// blocking sockets
	// also set a timeout on the reads to not wait
	// forever
	rawConn.Control(func(fd uintptr) {
		err = syscall.SetNonblock(int(fd), false)
		require.NoError(t, err, "could not set socket to blocking")
		err = syscall.SetsockoptTimeval(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &syscall.Timeval{Sec: 5})
		require.NoError(t, err, "could not set read timeout on socket")
	})

	read := 0
	buf := make([]byte, 6)
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		var n int
		readErr := rawConn.Read(func(fd uintptr) bool {
			n, _, err = syscall.Recvfrom(int(fd), buf[read:], syscall.MSG_WAITALL)
			return true
		})

		if !assert.NoError(collect, err, "error reading from connection") ||
			!assert.NoError(collect, readErr, "error from raw conn") {
			return
		}

		read += n
		t.Logf("read %d", read)
		assert.Equal(collect, 6, read)
	}, 10*time.Second, 100*time.Millisecond, "failed to get required bytes")

	var conn *network.ConnectionStats
	require.Eventually(t, func() bool {
		var found bool
		conn, found = findConnection(c.(*net.TCPConn).LocalAddr(), c.(*net.TCPConn).RemoteAddr(), getConnections(t, tr))
		return found
	}, 3*time.Second, 100*time.Millisecond)

	assert.Equal(t, uint64(read), conn.Monotonic.RecvBytes)
}

func (s *TracerSuite) TestPreexistingConnectionDirection() {
	t := s.T()
	// Start the client and server before we enable the system probe to test that the tracer picks
	// up the pre-existing connection

	server := tracertestutil.NewTCPServer(func(c net.Conn) {
		r := bufio.NewReader(c)
		for {
			if _, err := r.ReadBytes(byte('\n')); err != nil {
				assert.ErrorIs(t, err, io.EOF, "exited server loop with error that is not EOF")
				return
			}
			_, _ = c.Write(genPayload(serverMessageSize))
		}
	})
	t.Cleanup(server.Shutdown)
	require.NoError(t, server.Run())

	c, err := net.DialTimeout("tcp", server.Address(), 50*time.Millisecond)
	require.NoError(t, err)
	t.Cleanup(func() { c.Close() })

	_, err = c.Write(genPayload(clientMessageSize))
	require.NoError(t, err)
	r := bufio.NewReader(c)
	_, _ = r.ReadBytes(byte('\n'))

	// Enable BPF-based system probe
	tr := setupTracer(t, testConfig())
	// Write more data so that the tracer will notice the connection
	_, err = c.Write(genPayload(clientMessageSize))
	require.NoError(t, err)
	_, err = r.ReadBytes(byte('\n'))
	require.NoError(t, err)

	c.Close()

	var incoming, outgoing *network.ConnectionStats
	require.Eventually(t, func() bool {
		connections := getConnections(t, tr)
		if outgoing == nil {
			outgoing, _ = findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
		}
		if incoming == nil {
			incoming, _ = findConnection(c.RemoteAddr(), c.LocalAddr(), connections)
		}
		return incoming != nil && outgoing != nil
	}, 3*time.Second, 100*time.Millisecond, "could not find connection incoming and outgoing connections")

	m := outgoing.Monotonic
	assert.Equal(t, clientMessageSize, int(m.SentBytes))
	assert.Equal(t, serverMessageSize, int(m.RecvBytes))
	if !tr.config.EnableEbpfless {
		assert.Equal(t, os.Getpid(), int(outgoing.Pid))
	}
	assert.Equal(t, addrPort(server.Address()), int(outgoing.DPort))
	assert.Equal(t, c.LocalAddr().(*net.TCPAddr).Port, int(outgoing.SPort))
	assert.Equal(t, network.OUTGOING, outgoing.Direction)

	m = incoming.Monotonic
	assert.Equal(t, clientMessageSize, int(m.RecvBytes))
	assert.Equal(t, serverMessageSize, int(m.SentBytes))
	if !tr.config.EnableEbpfless {
		assert.Equal(t, os.Getpid(), int(incoming.Pid))
	}
	assert.Equal(t, addrPort(server.Address()), int(incoming.SPort))
	assert.Equal(t, c.LocalAddr().(*net.TCPAddr).Port, int(incoming.DPort))
	assert.Equal(t, network.INCOMING, incoming.Direction)
}

func (s *TracerSuite) TestPreexistingEmptyIncomingConnectionDirection() {
	t := s.T()
	t.Run("ringbuf_enabled", func(t *testing.T) {
		if features.HaveMapType(ebpf.RingBuf) != nil {
			t.Skip("skipping test as ringbuffers are not supported on this kernel")
		}
		c := testConfig()
		skipOnEbpflessNotSupported(t, c)
		c.NPMRingbuffersEnabled = true
		testPreexistingEmptyIncomingConnectionDirection(t, c)
	})
	t.Run("ringbuf_disabled", func(t *testing.T) {
		c := testConfig()
		c.NPMRingbuffersEnabled = false
		testPreexistingEmptyIncomingConnectionDirection(t, c)
	})
}

func testPreexistingEmptyIncomingConnectionDirection(t *testing.T, config *config.Config) {
	// Start the client and server before we enable the system probe to test that the tracer picks
	// up the pre-existing connection

	ch := make(chan struct{})
	server := tracertestutil.NewTCPServer(func(c net.Conn) {
		<-ch
		c.Close()
		close(ch)
	})
	require.NoError(t, server.Run())
	t.Cleanup(server.Shutdown)

	c, err := net.DialTimeout("tcp", server.Address(), 5*time.Second)
	require.NoError(t, err)

	// Enable BPF-based system probe
	tr := setupTracer(t, config)

	// close the server connection so the tracer picks it up
	ch <- struct{}{}
	<-ch

	var conn *network.ConnectionStats
	require.Eventually(t, func() bool {
		conns := getConnections(t, tr)
		t.Log(conns) // for debugging failures
		conn, _ = findConnection(c.RemoteAddr(), c.LocalAddr(), conns)
		return conn != nil
	}, 3*time.Second, 100*time.Millisecond)

	m := conn.Monotonic
	assert.Zero(t, m.SentBytes, "sent bytes should be 0")
	assert.Zero(t, m.RecvBytes, "recv bytes should be 0")
	assert.Zero(t, m.SentPackets, "sent packets should be 0")
	assert.Zero(t, m.RecvPackets, "recv packets should be 0")
	assert.Zero(t, m.TCPEstablished, "tcp established should be 0")
	assert.Equal(t, uint16(1), m.TCPClosed, "tcp closed should be 1")
	assert.Equal(t, network.INCOMING, conn.Direction, "connection direction should be incoming")
}

func (s *TracerSuite) TestUDPIncomingDirectionFix() {
	t := s.T()

	server := &UDPServer{
		network: "udp",
		address: "localhost:8125",
		onMessage: func(b []byte, _ int) []byte {
			return b
		},
	}

	cfg := testConfig()
	cfg.ProtocolClassificationEnabled = false
	tr := setupTracer(t, cfg)

	err := server.Run(64)
	require.NoError(t, err)
	t.Cleanup(server.Shutdown)

	sfd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, syscall.IPPROTO_UDP)
	require.NoError(t, err)
	t.Cleanup(func() { syscall.Close(sfd) })

	err = syscall.Bind(sfd, &syscall.SockaddrInet4{Addr: netip.MustParseAddr("127.0.0.1").As4()})
	require.NoError(t, err)

	err = syscall.Sendto(sfd, []byte("foo"), 0, &syscall.SockaddrInet4{Port: 8125, Addr: netip.MustParseAddr("127.0.0.1").As4()})
	require.NoError(t, err)

	_sa, err := syscall.Getsockname(sfd)
	require.NoError(t, err)
	sa := _sa.(*syscall.SockaddrInet4)
	ap := netip.AddrPortFrom(netip.AddrFrom4(sa.Addr), uint16(sa.Port))
	raddr, err := net.ResolveUDPAddr("udp", server.address)
	require.NoError(t, err)

	var conn *network.ConnectionStats
	require.Eventually(t, func() bool {
		conns := getConnections(t, tr)
		conn, _ = findConnection(net.UDPAddrFromAddrPort(ap), raddr, conns)
		return conn != nil
	}, 3*time.Second, 100*time.Millisecond)

	assert.Equal(t, network.OUTGOING, conn.Direction)
}

func TestEbpfConntrackerFallback(t *testing.T) {
	require.NoError(t, rlimit.RemoveMemlock())

	prebuiltErrorValues := []bool{true}
	if ebpfPrebuiltConntrackerSupportedOnKernelT(t) {
		prebuiltErrorValues = []bool{false, true}
	}
	coreErrorValues := []bool{true}
	if ebpfCOREConntrackerSupportedOnKernelT(t) {
		coreErrorValues = []bool{false, true}
	}

	type testCase struct {
		enableCORE            bool
		allowRuntimeFallback  bool
		enableRuntimeCompiler bool
		allowPrebuiltFallback bool
		coreError             bool
		rcError               bool
		prebuiltError         bool

		err        error
		isPrebuilt bool
	}

	var dtests []testCase
	for _, enableCORE := range []bool{false, true} {
		for _, allowRuntimeFallback := range []bool{false, true} {
			for _, enableRuntimeCompiler := range []bool{false, true} {
				for _, allowPrebuiltFallback := range []bool{false, true} {
					for _, coreError := range coreErrorValues {
						for _, rcError := range []bool{false, true} {
							for _, prebuiltError := range prebuiltErrorValues {
								tc := testCase{
									enableCORE:            enableCORE,
									allowRuntimeFallback:  allowRuntimeFallback,
									enableRuntimeCompiler: enableRuntimeCompiler,
									allowPrebuiltFallback: allowPrebuiltFallback,
									coreError:             coreError,
									rcError:               rcError,
									prebuiltError:         prebuiltError,

									isPrebuilt: !prebuiltError,
								}

								cerr := coreError
								if !enableCORE {
									cerr = true // not enabled, so assume always failed
								}

								rcEnabled := enableRuntimeCompiler
								rcerr := rcError
								if !enableRuntimeCompiler {
									rcerr = true // not enabled, so assume always failed
								}
								if enableCORE && !allowRuntimeFallback {
									rcEnabled = false
									rcerr = true // not enabled, so assume always failed
								}

								pberr := prebuiltError
								if (enableCORE || rcEnabled) && !allowPrebuiltFallback {
									pberr = true // not enabled, so assume always failed
									tc.isPrebuilt = false
								}

								if cerr && rcerr && pberr {
									tc.err = assert.AnError
									tc.isPrebuilt = false
								}

								if (enableCORE && !coreError) || (rcEnabled && !rcError) {
									tc.isPrebuilt = false
								}
								dtests = append(dtests, tc)
							}
						}
					}
				}
			}
		}
	}

	cfg := config.New()
	if kv >= kernel.VersionCode(5, 18, 0) {
		cfg.CollectUDPv6Conns = false
	}
	t.Cleanup(func() {
		ebpfConntrackerPrebuiltCreator = getPrebuiltConntracker
		ebpfConntrackerRCCreator = getRCConntracker
		ebpfConntrackerCORECreator = getCOREConntracker
	})

	for _, te := range dtests {
		t.Run("", func(t *testing.T) {
			t.Logf("%+v", te)

			cfg.EnableCORE = te.enableCORE
			cfg.AllowRuntimeCompiledFallback = te.allowRuntimeFallback
			cfg.EnableRuntimeCompiler = te.enableRuntimeCompiler
			cfg.AllowPrebuiltFallback = te.allowPrebuiltFallback

			ebpfConntrackerPrebuiltCreator = getPrebuiltConntracker
			ebpfConntrackerRCCreator = getRCConntracker
			ebpfConntrackerCORECreator = getCOREConntracker
			if te.prebuiltError {
				ebpfConntrackerPrebuiltCreator = func(_ *config.Config) (*manager.Manager, error) {
					return nil, assert.AnError
				}
			}
			if te.rcError {
				ebpfConntrackerRCCreator = func(_ *config.Config) (*manager.Manager, error) {
					return nil, assert.AnError
				}
			}
			if te.coreError {
				ebpfConntrackerCORECreator = func(_ *config.Config) (*manager.Manager, error) {
					return nil, assert.AnError
				}
			}

			conntracker, err := NewEBPFConntracker(cfg, nil)
			// ensure we always clean up the conntracker, regardless of behavior
			if conntracker != nil {
				t.Cleanup(conntracker.Close)
			}
			if te.err != nil {
				assert.Error(t, err)
				assert.Nil(t, conntracker)
				return
			}

			assert.NoError(t, err)
			require.NotNil(t, conntracker)
			assert.Equal(t, te.isPrebuilt, conntracker.(*ebpfConntracker).isPrebuilt, "is prebuilt")
		})
	}
}

func TestConntrackerFallback(t *testing.T) {
	cfg := testConfig()
	cfg.EnableEbpfConntracker = false
	conntracker, err := newConntracker(cfg, nil)
	// ensure we always clean up the conntracker, regardless of behavior
	if conntracker != nil {
		t.Cleanup(conntracker.Close)
	}
	assert.NoError(t, err)
	require.NotNil(t, conntracker)
	require.Equal(t, "netlink", conntracker.GetType())
}

func testConfig() *config.Config {
	cfg := config.New()
	if env.IsECSFargate() {
		// protocol classification not yet supported on fargate
		cfg.ProtocolClassificationEnabled = false
	}
	// prebuilt on 5.18+ does not support UDPv6
	if isPrebuilt(cfg) && kv >= kernel.VersionCode(5, 18, 0) {
		cfg.CollectUDPv6Conns = false
	}

	cfg.EnableGatewayLookup = false
	return cfg
}

func (s *TracerSuite) TestOffsetGuessIPv6DisabledCentOS() {
	t := s.T()
	cfg := testConfig()
	// disable IPv6 via config to trigger logic in GuessSocketSK
	cfg.CollectTCPv6Conns = false
	cfg.CollectUDPv6Conns = false
	kv, err := kernel.HostVersion()
	kv470 := kernel.VersionCode(4, 7, 0)
	require.NoError(t, err)
	if kv >= kv470 {
		// will only be run on kernels < 4.7.0 matching the GuessSocketSK check
		t.Skip("This test should only be run on kernels < 4.7.0")
	}
	// fail if tracer cannot start
	_ = setupTracer(t, cfg)
}

func BenchmarkAddProcessInfo(b *testing.B) {
	cfg := testConfig()
	cfg.EnableProcessEventMonitoring = true

	tr := setupTracer(b, cfg)
	var c network.ConnectionStats
	c.Pid = 1
	ts, err := ddebpf.NowNanoseconds()
	require.NoError(b, err)
	c.LastUpdateEpoch = uint64(ts)
	tr.processCache.add(&events.Process{
		Pid: 1,
		Tags: []*intern.Value{
			intern.GetByString("env:env"),
			intern.GetByString("version:version"),
			intern.GetByString("service:service"),
		},
		ContainerID: intern.GetByString("container"),
		StartTime:   time.Now().Unix(),
		Expiry:      time.Now().Add(5 * time.Minute).Unix(),
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Tags = nil
		tr.addProcessInfo(&c)
	}
}

func (s *TracerSuite) TestConnectionDuration() {
	t := s.T()
	cfg := testConfig()
	tr := setupTracer(t, cfg)

	srv := tracertestutil.NewTCPServer(func(c net.Conn) {
		var b [4]byte
		for {
			_, err := c.Read(b[:])
			if err != nil && (errors.Is(err, net.ErrClosed) || err == io.EOF) {
				return
			}
			require.NoError(t, err)
			_, err = c.Write([]byte("pong"))
			if err != nil && (errors.Is(err, net.ErrClosed) || err == io.EOF) {
				return
			}
			require.NoError(t, err)
		}
	})

	require.NoError(t, srv.Run(), "error running server")
	t.Cleanup(srv.Shutdown)

	srvAddr := srv.Address()
	c, err := net.DialTimeout("tcp", srvAddr, time.Second)
	require.NoError(t, err, "could not connect to server at %s", srvAddr)

	ticker := time.NewTicker(100 * time.Millisecond)
	t.Cleanup(ticker.Stop)

	timer := time.NewTimer(time.Second)
	t.Cleanup(func() { timer.Stop() })

LOOP:
	for {
		select {
		case <-timer.C:
			break LOOP
		case <-ticker.C:
			_, err = c.Write([]byte("ping"))
			require.NoError(t, err, "error writing ping to server")
			var b [4]byte
			_, err = c.Read(b[:])
			require.NoError(t, err, "error reading from server")
		}
	}

	// get connections, the client connection will still
	// not be in the closed state, so duration will the
	// timestamp of when it was created
	var conn *network.ConnectionStats
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		conns := getConnections(t, tr)
		var found bool
		conn, found = findConnection(c.LocalAddr(), srv.Addr(), conns)
		assert.True(collect, found, "could not find connection")

	}, 3*time.Second, 100*time.Millisecond, "could not find connection")
	// all we can do is verify it is > 0
	assert.Greater(t, conn.Duration, time.Duration(0))

	require.NoError(t, c.Close(), "error closing client connection")
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		var found bool
		conn, found = findConnection(c.LocalAddr(), srv.Addr(), getConnections(t, tr))
		assert.True(collect, found, "could not find closed connection")
	}, 3*time.Second, 100*time.Millisecond, "could not find closed connection")

	// after closing the client connection, the duration should be
	// updated to a value between 1s and 2s
	assert.Greater(t, conn.Duration, time.Second, "connection duration should be between 1 and 2 seconds")
	assert.Less(t, conn.Duration, 2*time.Second, "connection duration should be between 1 and 2 seconds")
}

var failedConnectionsBuildModes = map[ebpftest.BuildMode]struct{}{
	ebpftest.CORE:            {},
	ebpftest.RuntimeCompiled: {},
}

func checkSkipFailureConnectionsTests(t *testing.T) {
	if _, ok := failedConnectionsBuildModes[ebpftest.GetBuildMode()]; !ok {
		t.Skip("Skipping test on unsupported build mode: ", ebpftest.GetBuildMode())
	}
}

func (s *TracerSuite) TestTCPFailureConnectionTimeout() {
	t := s.T()

	checkSkipFailureConnectionsTests(t)
	// TODO: remove this check when we fix this test on kernels < 4.19
	if kv < kernel.VersionCode(4, 19, 0) {
		t.Skip("Skipping test on kernels < 4.19")
	}

	setupDropTrafficRule(t)
	cfg := testConfig()
	cfg.TCPFailedConnectionsEnabled = true
	tr := setupTracer(t, cfg)

	srvAddr := "127.0.0.1:10000"
	ipString, portString, err := net.SplitHostPort(srvAddr)
	require.NoError(t, err)
	ip := netip.MustParseAddr(ipString)
	port, err := strconv.Atoi(portString)
	require.NoError(t, err)

	addr := syscall.SockaddrInet4{
		Port: port,
		Addr: ip.As4(),
	}
	sfd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
	require.NoError(t, err)
	t.Cleanup(func() { syscall.Close(sfd) })

	//syscall.TCP_USER_TIMEOUT is 18 but not defined in our linter. Set it to 500ms
	err = syscall.SetsockoptInt(sfd, syscall.IPPROTO_TCP, 18, 500)
	require.NoError(t, err)

	err = syscall.Connect(sfd, &addr)
	if err != nil {
		var errno syscall.Errno
		if errors.As(err, &errno) && errors.Is(err, syscall.ETIMEDOUT) {
			t.Log("Connection timed out as expected")
		} else {
			require.NoError(t, err, "could not connect to server: ", err)
		}
	}

	f := os.NewFile(uintptr(sfd), "")
	defer f.Close()
	c, err := net.FileConn(f)
	require.NoError(t, err)
	port = c.LocalAddr().(*net.TCPAddr).Port
	// the addr here is 0.0.0.0, but the tracer sees it as 127.0.0.1
	localAddr := fmt.Sprintf("127.0.0.1:%d", port)

	// Check if the connection was recorded as failed due to timeout
	var conn *network.ConnectionStats
	require.Eventually(t, func() bool {
		conns := getConnections(t, tr)
		// 110 is the errno for ETIMEDOUT
		conn = findFailedConnection(t, localAddr, srvAddr, conns, 110)
		return conn != nil
	}, 3*time.Second, 100*time.Millisecond, "Failed connection not recorded properly")

	assert.Equal(t, uint32(0), conn.TCPFailures[104], "expected 0 connection reset")
	assert.Equal(t, uint32(0), conn.TCPFailures[111], "expected 0 connection refused")
	assert.Equal(t, uint32(1), conn.TCPFailures[110], "expected 1 connection timeout")
	assert.Equal(t, uint64(0), conn.Monotonic.SentBytes, "expected 0 bytes sent")
	assert.Equal(t, uint64(0), conn.Monotonic.RecvBytes, "expected 0 bytes received")
}

func (s *TracerSuite) TestTCPFailureConnectionResetWithDNAT() {
	t := s.T()

	checkSkipFailureConnectionsTests(t)

	cfg := testConfig()
	cfg.TCPFailedConnectionsEnabled = true
	tr := setupTracer(t, cfg)

	// Setup DNAT to redirect traffic from 2.2.2.2 to 1.1.1.1
	netlinktestutil.SetupDNAT(t)

	// Set up a TCP server on the translated address (1.1.1.1)
	srv := tracertestutil.NewTCPServerOnAddress("1.1.1.1:80", func(c net.Conn) {
		if tcpConn, ok := c.(*net.TCPConn); ok {
			tcpConn.SetLinger(0)
			buf := make([]byte, 10)
			_, _ = c.Read(buf)
			time.Sleep(10 * time.Millisecond)
		}
		c.Close()
	})

	require.NoError(t, srv.Run(), "error running server")
	t.Cleanup(srv.Shutdown)

	// Attempt to connect to the DNAT address (2.2.2.2), which should be redirected to the server at 1.1.1.1
	serverAddr := "2.2.2.2:80"
	c, err := net.Dial("tcp", serverAddr)
	require.NoError(t, err, "could not connect to server: ", err)

	// Write to the server and expect a reset
	_, writeErr := c.Write([]byte("ping"))
	if writeErr != nil {
		t.Log("Write error:", writeErr)
	}

	// Read from server to ensure that the server has a chance to reset the connection
	_, readErr := c.Read(make([]byte, 4))
	require.Error(t, readErr, "expected connection reset error but got none")

	// Check if the connection was recorded as reset
	var conn *network.ConnectionStats
	require.Eventually(t, func() bool {
		// 104 is the errno for ECONNRESET
		conn = findFailedConnection(t, c.LocalAddr().String(), serverAddr, getConnections(t, tr), 104)
		return conn != nil
	}, 3*time.Second, 100*time.Millisecond, "Failed connection not recorded properly")

	require.NoError(t, c.Close(), "error closing client connection")
	assert.Equal(t, uint32(1), conn.TCPFailures[104], "expected 1 connection reset")
	assert.Equal(t, uint32(0), conn.TCPFailures[111], "expected 0 connection refused")
	assert.Equal(t, uint32(0), conn.TCPFailures[110], "expected 0 connection timeout")
	assert.Equal(t, uint64(4), conn.Monotonic.SentBytes, "expected 4 bytes sent")
	assert.Equal(t, uint64(0), conn.Monotonic.RecvBytes, "expected 0 bytes received")
}

func setupDropTrafficRule(tb testing.TB) (ns string) {
	state := testutil.IptablesSave(tb)
	tb.Cleanup(func() {
		testutil.IptablesRestore(tb, state)
	})
	cmds := []string{
		"iptables -A OUTPUT -p tcp -d 127.0.0.1 --dport 10000 -j DROP",
	}
	testutil.RunCommands(tb, cmds, false)
	return
}

func skipOnEbpflessNotSupported(t *testing.T, cfg *config.Config) {
	if cfg.EnableEbpfless {
		t.Skip("not supported on ebpf-less")
	}
}

func skipEbpflessTodo(t *testing.T, cfg *config.Config) {
	if cfg.EnableEbpfless {
		t.Skip("TODO: ebpf-less")
	}
}
