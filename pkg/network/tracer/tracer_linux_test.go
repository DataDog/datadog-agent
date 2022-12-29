// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package tracer

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	vnetns "github.com/vishvananda/netns"
	"golang.org/x/sys/unix"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/config/sysctl"
	"github.com/DataDog/datadog-agent/pkg/network/testutil"
	tracertest "github.com/DataDog/datadog-agent/pkg/network/tracer/testutil"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func doDNSQuery(t *testing.T, domain string, serverIP string) (*net.UDPAddr, *net.UDPAddr) {
	dnsServerAddr := &net.UDPAddr{IP: net.ParseIP(serverIP), Port: 53}
	queryMsg := new(dns.Msg)
	queryMsg.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	queryMsg.RecursionDesired = true
	dnsClient := new(dns.Client)
	dnsConn, err := dnsClient.Dial(dnsServerAddr.String())
	require.NoError(t, err)
	defer dnsConn.Close()
	dnsClientAddr := dnsConn.LocalAddr().(*net.UDPAddr)
	_, _, err = dnsClient.ExchangeWithConn(queryMsg, dnsConn)
	require.NoError(t, err)

	return dnsClientAddr, dnsServerAddr
}

func TestTCPRemoveEntries(t *testing.T) {
	config := testConfig()
	config.TCPConnTimeout = 100 * time.Millisecond
	tr, err := NewTracer(config)
	require.NoError(t, err)
	defer tr.Stop()

	// Create a dummy TCP Server
	server := NewTCPServer(func(c net.Conn) {
		c.Close()
	})
	doneChan := make(chan struct{})
	err = server.Run(doneChan)
	require.NoError(t, err)
	defer close(doneChan)

	// Connect to server
	c, err := net.DialTimeout("tcp", server.address, 2*time.Second)
	require.NoError(t, err)

	// Write a message
	_, err = c.Write(genPayload(clientMessageSize))
	require.NoError(t, err)
	defer c.Close()

	// Write a bunch of messages with blocking iptable rule to create retransmits
	iptablesWrapper(t, func() {
		for i := 0; i < 99; i++ {
			// Send a bunch of messages
			c.Write(genPayload(clientMessageSize))
		}
		time.Sleep(time.Second)
	})

	// Wait a bit for the first connection to be considered as timeouting
	time.Sleep(1 * time.Second)

	// Create a new client
	c2, err := net.DialTimeout("tcp", server.address, 1*time.Second)
	require.NoError(t, err)

	// Send a messages
	_, err = c2.Write(genPayload(clientMessageSize))
	require.NoError(t, err)
	defer c2.Close()

	c.Close()
	// Retrieve the list of connections
	connections := getConnections(t, tr)

	// Make sure the first connection got cleaned up
	_, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	assert.False(t, ok)

	conn, ok := findConnection(c2.LocalAddr(), c2.RemoteAddr(), connections)
	require.True(t, ok)
	assert.Equal(t, clientMessageSize, int(conn.Monotonic.SentBytes))
	assert.Equal(t, 0, int(conn.Monotonic.RecvBytes))
	assert.Equal(t, 0, int(conn.Monotonic.Retransmits))
	assert.Equal(t, os.Getpid(), int(conn.Pid))
	assert.Equal(t, addrPort(server.address), int(conn.DPort))
}

func TestTCPRetransmit(t *testing.T) {
	// Enable BPF-based system probe
	tr, err := NewTracer(testConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	// Create TCP Server which sends back serverMessageSize bytes
	server := NewTCPServer(func(c net.Conn) {
		r := bufio.NewReader(c)
		r.ReadBytes(byte('\n'))
		c.Write(genPayload(serverMessageSize))
		c.Close()
	})
	doneChan := make(chan struct{})
	err = server.Run(doneChan)
	require.NoError(t, err)
	defer close(doneChan)

	// Connect to server
	c, err := net.DialTimeout("tcp", server.address, time.Second)
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
	assert.Equal(t, addrPort(server.address), int(conn.DPort))
}

func TestTCPRetransmitSharedSocket(t *testing.T) {
	// Enable BPF-based system probe
	tr, err := NewTracer(testConfig())
	require.NoError(t, err)
	defer tr.Stop()

	// Create TCP Server that simply "drains" connection until receiving an EOF
	server := NewTCPServer(func(c net.Conn) {
		io.Copy(io.Discard, c)
		c.Close()
	})
	doneChan := make(chan struct{})
	err = server.Run(doneChan)
	require.NoError(t, err)
	defer close(doneChan)

	// Connect to server
	c, err := net.DialTimeout("tcp", server.address, time.Second)
	require.NoError(t, err)
	socketFile, err := c.(*net.TCPConn).File()
	require.NoError(t, err)
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
	_ = socketFile.Close()
	c.Close()

	// Fetch all connections matching source and target address
	allConnections := getConnections(t, tr)
	conns := searchConnections(allConnections, byAddress(c.LocalAddr(), c.RemoteAddr()))
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

	telemetry := tr.ebpfTracer.GetTelemetry()
	// Test if telemetry measuring PID collisions is correct
	// >= because there can be other connections going on during CI that increase pidCollisions
	assert.GreaterOrEqual(t, telemetry["pid_collisions"], int64(numProcesses-1))
}

func TestTCPRTT(t *testing.T) {
	// Enable BPF-based system probe
	tr, err := NewTracer(testConfig())
	require.NoError(t, err)
	defer tr.Stop()

	// Create TCP Server that simply "drains" connection until receiving an EOF
	server := NewTCPServer(func(c net.Conn) {
		io.Copy(io.Discard, c)
		c.Close()
	})
	doneChan := make(chan struct{})
	err = server.Run(doneChan)
	require.NoError(t, err)
	defer close(doneChan)

	c, err := net.DialTimeout("tcp", server.address, time.Second)
	require.NoError(t, err)
	defer c.Close()

	// Wait for a second so RTT can stabilize
	time.Sleep(1 * time.Second)

	// Write something to socket to ensure connection is tracked
	// This will trigger the collection of TCP stats including RTT
	_, err = c.Write([]byte("foo"))
	require.NoError(t, err)

	// Obtain information from a TCP socket via GETSOCKOPT(2) system call.
	tcpInfo, err := tcpGetInfo(c)
	require.NoError(t, err)

	// Fetch connection matching source and target address
	allConnections := getConnections(t, tr)
	conn, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), allConnections)
	require.True(t, ok)

	// Assert that values returned from syscall match ones generated by eBPF program
	assert.EqualValues(t, int(tcpInfo.Rtt), int(conn.RTT))
	assert.EqualValues(t, int(tcpInfo.Rttvar), int(conn.RTTVar))
}

func TestTCPMiscount(t *testing.T) {
	t.Skip("skipping because this test will pass/fail depending on host performance")
	tr, err := NewTracer(testConfig())
	require.NoError(t, err)
	defer tr.Stop()

	// Create a dummy TCP Server
	server := NewTCPServer(func(c net.Conn) {
		r := bufio.NewReader(c)
		for {
			if _, err := r.ReadBytes(byte('\n')); err != nil { // indicates that EOF has been reached,
				break
			}
		}
		c.Close()
	})
	doneChan := make(chan struct{})
	err = server.Run(doneChan)
	require.NoError(t, err)
	defer close(doneChan)

	c, err := net.DialTimeout("tcp", server.address, 50*time.Millisecond)
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

	doneChan <- struct{}{}

	conn, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), getConnections(t, tr))
	assert.True(t, ok)

	// TODO this should not happen but is expected for now
	// we don't have the correct count since retries happened
	assert.False(t, uint64(len(x)) == conn.Monotonic.SentBytes)

	tel := tr.ebpfTracer.GetTelemetry()
	assert.NotZero(t, tel["tcp_sent_miscounts"])
}

func TestConnectionExpirationRegression(t *testing.T) {
	t.SkipNow()
	tr, err := NewTracer(testConfig())
	require.NoError(t, err)
	defer tr.Stop()

	// Create TCP Server that simply "drains" connection until receiving an EOF
	connClosed := make(chan struct{})
	server := NewTCPServer(func(c net.Conn) {
		io.Copy(io.Discard, c)
		c.Close()
		connClosed <- struct{}{}
	})
	doneChan := make(chan struct{})
	err = server.Run(doneChan)
	require.NoError(t, err)
	defer close(doneChan)

	c, err := net.DialTimeout("tcp", server.address, time.Second)
	require.NoError(t, err)

	// Warm up state
	_ = getConnections(t, tr)

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

func TestConntrackExpiration(t *testing.T) {
	setupDNAT(t)

	tr, err := NewTracer(testConfig())
	require.NoError(t, err)
	defer tr.Stop()

	// Warm-up tracer state
	_ = getConnections(t, tr)

	// The random port is necessary to avoid flakiness in the test. Running the the test multiple
	// times can fail if binding to the same port since Conntrack might not emit NEW events for the same tuple
	rand.Seed(time.Now().UnixNano())
	port := 5430 + rand.Intn(100)
	server := NewTCPServerOnAddress(fmt.Sprintf("1.1.1.1:%d", port), func(c net.Conn) {
		io.Copy(io.Discard, c)
		c.Close()
	})
	doneChan := make(chan struct{})
	err = server.Run(doneChan)
	require.NoError(t, err)
	defer close(doneChan)

	c, err := net.Dial("tcp", fmt.Sprintf("2.2.2.2:%d", port))
	require.NoError(t, err)
	defer c.Close()
	_, err = c.Write([]byte("ping"))
	require.NoError(t, err)

	// Give enough time for conntrack cache to be populated
	time.Sleep(100 * time.Millisecond)

	connections := getConnections(t, tr)
	conn, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	require.True(t, ok)
	require.NotNil(t, tr.conntracker.GetTranslationForConn(*conn), "missing translation for connection")

	// This will force the connection to be expired next time we call getConnections, but
	// conntrack should still have the connection information since the connection is still
	// alive
	tr.config.TCPConnTimeout = time.Duration(-1)
	_ = getConnections(t, tr)

	assert.NotNil(t, tr.conntracker.GetTranslationForConn(*conn), "translation should not have been deleted")

	// delete the connection from system conntrack
	cmd := exec.Command("conntrack", "-D", "-s", c.LocalAddr().(*net.TCPAddr).IP.String(), "-d", c.RemoteAddr().(*net.TCPAddr).IP.String(), "-p", "tcp")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "conntrack delete failed, output: %s", out)
	_ = getConnections(t, tr)

	assert.Nil(t, tr.conntracker.GetTranslationForConn(*conn), "translation should have been deleted")
}

// This test ensures that conntrack lookups are retried for short-lived
// connections when the first lookup fails
func TestConntrackDelays(t *testing.T) {
	setupDNAT(t)

	tr, err := NewTracer(testConfig())
	require.NoError(t, err)
	defer tr.Stop()

	// This will ensure that the first lookup for every connection fails, while the following ones succeed
	tr.conntracker = tracertest.NewDelayedConntracker(tr.conntracker, 1)

	// Warm-up tracer state
	_ = getConnections(t, tr)

	// The random port is necessary to avoid flakiness in the test. Running the the test multiple
	// times can fail if binding to the same port since Conntrack might not emit NEW events for the same tuple
	rand.Seed(time.Now().UnixNano())
	port := 5430 + rand.Intn(100)
	server := NewTCPServerOnAddress(fmt.Sprintf("1.1.1.1:%d", port), func(c net.Conn) {
		io.Copy(io.Discard, c)
		c.Close()
	})
	doneChan := make(chan struct{})
	err = server.Run(doneChan)
	require.NoError(t, err)
	defer close(doneChan)

	c, err := net.Dial("tcp", fmt.Sprintf("2.2.2.2:%d", port))
	require.NoError(t, err)
	defer c.Close()
	_, err = c.Write([]byte("ping"))
	require.NoError(t, err)

	// Give enough time for conntrack cache to be populated
	time.Sleep(100 * time.Millisecond)

	connections := getConnections(t, tr)
	conn, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	require.True(t, ok)
	require.NotNil(t, tr.conntracker.GetTranslationForConn(*conn), "missing translation for connection")
}

func TestTranslationBindingRegression(t *testing.T) {
	setupDNAT(t)

	tr, err := NewTracer(testConfig())
	require.NoError(t, err)
	defer tr.Stop()

	// Warm-up tracer state
	_ = getConnections(t, tr)

	// Setup TCP server
	rand.Seed(time.Now().UnixNano())
	port := 5430 + rand.Intn(100)
	server := NewTCPServerOnAddress(fmt.Sprintf("1.1.1.1:%d", port), func(c net.Conn) {
		io.Copy(io.Discard, c)
		c.Close()
	})
	doneChan := make(chan struct{})
	err = server.Run(doneChan)
	require.NoError(t, err)
	defer close(doneChan)

	// Send data to 2.2.2.2 (which should be translated to 1.1.1.1)
	c, err := net.Dial("tcp", fmt.Sprintf("2.2.2.2:%d", port))
	require.NoError(t, err)
	defer c.Close()
	_, err = c.Write([]byte("ping"))
	require.NoError(t, err)

	// Give enough time for conntrack cache to be populated
	time.Sleep(100 * time.Millisecond)

	// Assert that the connection to 2.2.2.2 has an IPTranslation object bound to it
	connections := getConnections(t, tr)
	conn, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	require.True(t, ok)
	require.NotNil(t, conn.IPTranslation, "missing translation for connection")
}

func TestUnconnectedUDPSendIPv6(t *testing.T) {
	if !kernel.IsIPv6Enabled() {
		t.Skip("IPv6 not enabled on host")
	}

	cfg := testConfig()
	cfg.CollectIPv6Conns = true
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	linkLocal, err := getIPv6LinkLocalAddress()
	require.NoError(t, err)

	remotePort := rand.Int()%5000 + 15000
	remoteAddr := &net.UDPAddr{IP: net.ParseIP(interfaceLocalMulticastIPv6), Port: remotePort}
	conn, err := net.ListenUDP("udp6", linkLocal)
	require.NoError(t, err)
	defer conn.Close()
	message := []byte("payload")
	bytesSent, err := conn.WriteTo(message, remoteAddr)
	require.NoError(t, err)

	connections := getConnections(t, tr)
	outgoing := searchConnections(connections, func(cs network.ConnectionStats) bool {
		return cs.DPort == uint16(remotePort)
	})

	require.Len(t, outgoing, 1)
	assert.Equal(t, remoteAddr.IP.String(), outgoing[0].Dest.String())
	assert.Equal(t, bytesSent, int(outgoing[0].Monotonic.SentBytes))
}

func TestGatewayLookupNotEnabled(t *testing.T) {
	t.Run("gateway lookup enabled, not on aws", func(t *testing.T) {
		cfg := testConfig()
		cfg.EnableGatewayLookup = true
		oldCloud := cloud
		defer func() {
			cloud = oldCloud
		}()
		ctrl := gomock.NewController(t)
		m := NewMockcloudProvider(ctrl)
		m.EXPECT().IsAWS().Return(false)
		cloud = m
		tr, err := NewTracer(cfg)
		require.NoError(t, err)
		require.NotNil(t, tr)
		defer tr.Stop()
		require.Nil(t, tr.gwLookup)
	})

	t.Run("gateway lookup enabled, aws metadata endpoint not enabled", func(t *testing.T) {
		cfg := testConfig()
		cfg.EnableGatewayLookup = true
		oldCloud := cloud
		defer func() {
			cloud = oldCloud
		}()
		ctrl := gomock.NewController(t)
		m := NewMockcloudProvider(ctrl)
		m.EXPECT().IsAWS().Return(true)
		cloud = m

		clouds := ddconfig.Datadog.Get("cloud_provider_metadata")
		ddconfig.Datadog.Set("cloud_provider_metadata", []string{})
		defer ddconfig.Datadog.Set("cloud_provider_metadata", clouds)

		tr, err := NewTracer(cfg)
		require.NoError(t, err)
		require.NotNil(t, tr)
		defer tr.Stop()
		require.Nil(t, tr.gwLookup)
	})
}

func TestGatewayLookupEnabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := NewMockcloudProvider(ctrl)
	oldCloud := cloud
	defer func() {
		cloud = oldCloud
	}()

	m.EXPECT().IsAWS().Return(true)
	cloud = m

	ifi := ipRouteGet(t, "", "8.8.8.8", nil)
	ifs, err := net.Interfaces()
	require.NoError(t, err)

	cfg := testConfig()
	cfg.EnableGatewayLookup = true
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	require.NotNil(t, tr)
	defer tr.Stop()
	require.NotNil(t, tr.gwLookup)

	tr.gwLookup.subnetForHwAddrFunc = func(hwAddr net.HardwareAddr) (network.Subnet, error) {
		t.Logf("subnet lookup: %s", hwAddr)
		for _, i := range ifs {
			if hwAddr.String() == i.HardwareAddr.String() {
				return network.Subnet{Alias: fmt.Sprintf("subnet-%d", i.Index)}, nil
			}
		}

		return network.Subnet{Alias: "subnet"}, nil
	}

	getConnections(t, tr)

	dnsClientAddr, dnsServerAddr := doDNSQuery(t, "google.com", "8.8.8.8")

	var conn *network.ConnectionStats
	require.Eventually(t, func() bool {
		var ok bool
		conn, ok = findConnection(dnsClientAddr, dnsServerAddr, getConnections(t, tr))
		return ok
	}, 2*time.Second, time.Second)

	require.NotNil(t, conn.Via, "connection is missing via: %s", conn)
	require.Equal(t, conn.Via.Subnet.Alias, fmt.Sprintf("subnet-%d", ifi.Index))
}

func TestGatewayLookupSubnetLookupError(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := NewMockcloudProvider(ctrl)
	oldCloud := cloud
	defer func() {
		cloud = oldCloud
	}()

	m.EXPECT().IsAWS().Return(true)
	cloud = m

	cfg := testConfig()
	cfg.EnableGatewayLookup = true
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	require.NotNil(t, tr)
	defer tr.Stop()

	require.NotNil(t, tr.gwLookup)

	ifi := ipRouteGet(t, "", "8.8.8.8", nil)
	calls := 0
	tr.gwLookup.subnetForHwAddrFunc = func(hwAddr net.HardwareAddr) (network.Subnet, error) {
		if hwAddr.String() == ifi.HardwareAddr.String() {
			calls++
		}
		return network.Subnet{}, assert.AnError
	}

	tr.gwLookup.purge()

	getConnections(t, tr)

	// do two dns queries to prompt more than one subnet lookup attempt
	localAddr, remoteAddr := doDNSQuery(t, "google.com", "8.8.8.8")
	var c *network.ConnectionStats
	require.Eventually(t, func() bool {
		var ok bool
		c, ok = findConnection(localAddr, remoteAddr, getConnections(t, tr))
		return ok
	}, 3*time.Second, 500*time.Millisecond, "connection not found")
	require.Nil(t, c.Via)

	localAddr, remoteAddr = doDNSQuery(t, "google.com", "8.8.8.8")
	require.Eventually(t, func() bool {
		var ok bool
		c, ok = findConnection(localAddr, remoteAddr, getConnections(t, tr))
		return ok
	}, 3*time.Second, 500*time.Millisecond, "connection not found")
	require.Nil(t, c.Via)

	require.Equal(t, 1, calls, "calls to subnetForHwAddrFunc are != 1 for hw addr %s", ifi.HardwareAddr)
}

func TestGatewayLookupCrossNamespace(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := NewMockcloudProvider(ctrl)
	oldCloud := cloud
	defer func() {
		cloud = oldCloud
	}()

	m.EXPECT().IsAWS().Return(true)
	cloud = m

	cfg := testConfig()
	cfg.EnableGatewayLookup = true
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	require.NotNil(t, tr)
	defer tr.Stop()

	require.NotNil(t, tr.gwLookup)

	// setup two network namespaces
	state := testutil.IptablesSave(t)
	cmds := []string{
		"ip link add br0 type bridge",
		"ip addr add 2.2.2.1/24 broadcast 2.2.2.255 dev br0",
		"ip netns add test1",
		"ip netns add test2",
		"ip link add veth1 type veth peer name veth2",
		"ip link set veth1 master br0",
		"ip link set veth2 netns test1",
		"ip -n test1 addr add 2.2.2.2/24 broadcast 2.2.2.255 dev veth2",
		"ip link add veth3 type veth peer name veth4",
		"ip link set veth3 master br0",
		"ip link set veth4 netns test2",
		"ip -n test2 addr add 2.2.2.3/24 broadcast 2.2.2.255 dev veth4",
		"ip link set br0 up",
		"ip link set veth1 up",
		"ip -n test1 link set veth2 up",
		"ip link set veth3 up",
		"ip -n test2 link set veth4 up",
		"ip -n test1 r add default via 2.2.2.1",
		"ip -n test2 r add default via 2.2.2.1",
		"iptables -I POSTROUTING 1 -t nat -s 2.2.2.0/24 ! -d 2.2.2.0/24 -j MASQUERADE",
		"iptables -I FORWARD -i br0 -j ACCEPT",
		"iptables -I FORWARD -o br0 -j ACCEPT",
		"sysctl -w net.ipv4.ip_forward=1",
	}
	t.Cleanup(func() {
		testutil.IptablesRestore(t, state)
		testutil.RunCommands(t, []string{
			"ip link del veth1",
			"ip link del veth3",
			"ip link del br0",
			"ip netns del test1",
			"ip netns del test2",
		}, true)
	})

	testutil.RunCommands(t, cmds, false)

	ifs, err := net.Interfaces()
	require.NoError(t, err)
	tr.gwLookup.subnetForHwAddrFunc = func(hwAddr net.HardwareAddr) (network.Subnet, error) {
		for _, i := range ifs {
			if hwAddr.String() == i.HardwareAddr.String() {
				return network.Subnet{Alias: fmt.Sprintf("subnet-%s", i.Name)}, nil
			}
		}

		return network.Subnet{Alias: "subnet"}, nil
	}
	tr.gwLookup.purge()

	test1Ns, err := vnetns.GetFromName("test1")
	require.NoError(t, err)
	defer test1Ns.Close()

	// run tcp server in test1 net namespace
	done := make(chan struct{})
	var server *TCPServer
	err = util.WithNS(test1Ns, func() error {
		server = NewTCPServerOnAddress("2.2.2.2:0", func(c net.Conn) {})
		return server.Run(done)
	})
	require.NoError(t, err)
	defer close(done)

	var conn *network.ConnectionStats
	t.Run("client in root namespace", func(t *testing.T) {
		c, err := net.DialTimeout("tcp", server.address, 2*time.Second)
		require.NoError(t, err)

		// write some data
		_, err = c.Write([]byte("foo"))
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			var ok bool
			conn, ok = findConnection(c.LocalAddr(), c.RemoteAddr(), getConnections(t, tr))
			return ok && conn.Direction == network.OUTGOING
		}, 2*time.Second, 500*time.Millisecond)

		// conn.Via should be nil, since traffic is local
		require.Nil(t, conn.Via)
	})

	t.Run("client in other namespace", func(t *testing.T) {
		// try connecting to server in test1 namespace
		test2Ns, err := vnetns.GetFromName("test2")
		require.NoError(t, err)
		defer test2Ns.Close()

		var c net.Conn
		err = util.WithNS(test2Ns, func() error {
			var err error
			c, err = net.DialTimeout("tcp", server.address, 2*time.Second)
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
			conn, ok = findConnection(c.LocalAddr(), c.RemoteAddr(), getConnections(t, tr))
			return ok && conn.Direction == network.OUTGOING
		}, 2*time.Second, 500*time.Millisecond)

		// traffic is local, so Via field should not be set
		require.Nil(t, conn.Via)

		// try connecting to something outside
		var dnsClientAddr, dnsServerAddr *net.UDPAddr
		util.WithNS(test2Ns, func() error {
			dnsClientAddr, dnsServerAddr = doDNSQuery(t, "google.com", "8.8.8.8")
			return nil
		})

		iif := ipRouteGet(t, "", dnsClientAddr.IP.String(), nil)
		ifi := ipRouteGet(t, dnsClientAddr.IP.String(), "8.8.8.8", iif)

		require.Eventually(t, func() bool {
			var ok bool
			conn, ok = findConnection(dnsClientAddr, dnsServerAddr, getConnections(t, tr))
			return ok && conn.Direction == network.OUTGOING
		}, 3*time.Second, 500*time.Millisecond)

		require.NotNil(t, conn.Via)
		require.Equal(t, fmt.Sprintf("subnet-%s", ifi.Name), conn.Via.Subnet.Alias)

	})
}

func TestConnectionAssured(t *testing.T) {
	cfg := testConfig()
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	require.NotNil(t, tr)
	defer tr.Stop()

	// register test as client
	getConnections(t, tr)

	server := &UDPServer{
		onMessage: func(b []byte, n int) []byte {
			return genPayload(serverMessageSize)
		},
	}

	done := make(chan struct{})
	err = server.Run(done, clientMessageSize)
	require.NoError(t, err)
	defer close(done)

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
		m := conn.Monotonic
		return ok && m.SentBytes > 0 && m.RecvBytes > 0
	}, 3*time.Second, 500*time.Millisecond, "could not find udp connection")

	// verify the connection is marked as assured
	require.True(t, conn.IsAssured)
}

func TestConnectionNotAssured(t *testing.T) {
	cfg := testConfig()
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	require.NotNil(t, tr)
	defer tr.Stop()

	// register test as client
	getConnections(t, tr)

	server := &UDPServer{
		onMessage: func(b []byte, n int) []byte {
			return nil
		},
	}

	done := make(chan struct{})
	err = server.Run(done, clientMessageSize)
	require.NoError(t, err)
	defer close(done)

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
		m := conn.Monotonic
		return ok && m.SentBytes > 0 && m.RecvBytes == 0
	}, 3*time.Second, 500*time.Millisecond, "could not find udp connection")

	// verify the connection is marked as not assured
	require.False(t, conn.IsAssured)
}

func TestUDPConnExpiryTimeout(t *testing.T) {
	streamTimeout, err := sysctl.NewInt("/proc", "net/netfilter/nf_conntrack_udp_timeout_stream", 0).Get()
	require.NoError(t, err)
	timeout, err := sysctl.NewInt("/proc", "net/netfilter/nf_conntrack_udp_timeout", 0).Get()
	require.NoError(t, err)

	tr, err := NewTracer(testConfig())
	require.NoError(t, err)
	defer tr.Stop()

	require.Equal(t, uint64(time.Duration(timeout)*time.Second), tr.udpConnTimeout(false))
	require.Equal(t, uint64(time.Duration(streamTimeout)*time.Second), tr.udpConnTimeout(true))
}

func TestDNATIntraHostIntegration(t *testing.T) {
	t.SkipNow()

	setupDNAT(t)

	tr, err := NewTracer(testConfig())
	require.NoError(t, err)
	defer tr.Stop()

	_ = getConnections(t, tr).Conns

	server := &TCPServer{
		address: "1.1.1.1:5432",
		onMessage: func(c net.Conn) {
			bs := make([]byte, 1)
			_, err := c.Read(bs)
			require.NoError(t, err, "error reading in server")

			_, err = c.Write([]byte("Ping back"))
			require.NoError(t, err, "error writing back in server")
			_ = c.Close()
		},
	}
	doneChan := make(chan struct{})
	err = server.Run(doneChan)
	require.NoError(t, err)
	defer close(doneChan)

	conn, err := net.Dial("tcp", "2.2.2.2:5432")
	require.NoError(t, err, "error connecting to client")
	_, err = conn.Write([]byte("ping"))
	require.NoError(t, err, "error writing in client")

	bs := make([]byte, 1)
	_, err = conn.Read(bs)
	require.NoError(t, err)
	require.NoError(t, conn.Close(), "error closing client connection")

	doneChan <- struct{}{}

	time.Sleep(time.Second * 1)

	conns := getConnections(t, tr).Conns
	assert.Condition(t, func() bool {
		for _, c := range conns {
			if c.Source == util.AddressFromString("1.1.1.1") {
				return c.IntraHost == true
			}
		}

		return false
	}, "did not find 1.1.1.1 connection classified as local: %v", conns)

	assert.Condition(t, func() bool {
		for _, c := range conns {
			if c.Dest == util.AddressFromString("2.2.2.2") {
				return c.IntraHost == true
			}
		}
		return true
	})
}

func TestSelfConnect(t *testing.T) {
	// Enable BPF-based system probe
	cfg := testConfig()
	cfg.TCPConnTimeout = 3 * time.Second
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	getConnections(t, tr)

	started := make(chan struct{})
	cmd := exec.Command("testdata/fork.py")
	stdOutReader, stdOutWriter := io.Pipe()
	go func() {
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		cmd.Stdout = stdOutWriter
		err := cmd.Start()
		close(started)
		require.NoError(t, err)
		if err := cmd.Wait(); err != nil {
			status := cmd.ProcessState.Sys().(syscall.WaitStatus)
			require.Equal(t, syscall.SIGKILL, status.Signal(), "fork.py output: %s", stderr.String())
		}
	}()

	<-started

	defer cmd.Process.Kill()

	portStr, err := bufio.NewReader(stdOutReader).ReadString('\n')
	require.NoError(t, err, "error reading port from fork.py")
	stdOutReader.Close()

	port, err := strconv.ParseUint(strings.TrimSpace(portStr), 10, 16)
	require.NoError(t, err, "could not convert %s to integer port", portStr)

	t.Logf("port is %d", port)

	require.Eventually(t, func() bool {
		conns := searchConnections(getConnections(t, tr), func(cs network.ConnectionStats) bool {
			return cs.SPort == uint16(port) && cs.DPort == uint16(port) && cs.Source.IsLoopback() && cs.Dest.IsLoopback()
		})

		t.Logf("connections: %v", conns)
		return len(conns) == 2
	}, 5*time.Second, time.Second, "could not find expected number of tcp connections, expected: 2")
}

func TestUDPPeekCount(t *testing.T) {
	config := testConfig()
	tr, err := NewTracer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	getConnections(t, tr)

	ln, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	saddr := ln.LocalAddr().String()

	laddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	raddr, err := net.ResolveUDPAddr("udp", saddr)
	require.NoError(t, err)

	c, err := net.DialUDP("udp", laddr, raddr)
	require.NoError(t, err)
	defer c.Close()

	msg := []byte("asdf")
	_, err = c.Write(msg)
	require.NoError(t, err)

	rawConn, err := ln.(*net.UDPConn).SyscallConn()
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
	require.True(t, outgoing.IntraHost)

	// make sure the inverse values are seen for the other message
	m = incoming.Monotonic
	require.Equal(t, 0, int(m.SentBytes))
	require.Equal(t, len(msg), int(m.RecvBytes))
	require.True(t, incoming.IntraHost)
}

func TestUDPPythonReusePort(t *testing.T) {
	cfg := testConfig()
	if !cfg.EnableRuntimeCompiler {
		t.Skip("reuseport only supported on runtime compilation")
	}

	cfg.TCPConnTimeout = 3 * time.Second
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	t.Cleanup(tr.Stop)

	started := make(chan struct{})
	cmd := exec.Command("testdata/reuseport.py")
	stdOutReader, stdOutWriter := io.Pipe()
	go func() {
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		cmd.Stdout = stdOutWriter
		err := cmd.Start()
		close(started)
		require.NoError(t, err)
		cmd.Wait()
	}()

	<-started

	defer cmd.Process.Kill()

	portStr, err := bufio.NewReader(stdOutReader).ReadString('\n')
	require.NoError(t, err, "error reading port from fork.py")
	stdOutReader.Close()
	port, err := strconv.ParseUint(strings.TrimSpace(portStr), 10, 16)
	require.NoError(t, err, "could not convert %s to integer port", portStr)

	t.Logf("port is %d", port)

	var conns []network.ConnectionStats
	require.Eventually(t, func() bool {
		conns = searchConnections(getConnections(t, tr), func(cs network.ConnectionStats) bool {
			return cs.Type == network.UDP &&
				cs.Source.IsLoopback() &&
				cs.Dest.IsLoopback() &&
				(cs.DPort == uint16(port) || cs.SPort == uint16(port))
		})

		return len(conns) == 4
	}, 5*time.Second, time.Second, "could not find expected number of udp connections, expected: 4")

	var incoming, outgoing []network.ConnectionStats
	for _, c := range conns {
		t.Log(c)
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

func TestUDPReusePort(t *testing.T) {
	t.Run("v4", func(t *testing.T) {
		testUDPReusePort(t, "udp4", "127.0.0.1")
	})
	t.Run("v6", func(t *testing.T) {
		testUDPReusePort(t, "udp6", "[::1]")
	})
}

func testUDPReusePort(t *testing.T, udpnet string, ip string) {
	cfg := testConfig()
	if !cfg.EnableRuntimeCompiler {
		t.Skip("reuseport only supported on runtime compilation")
	}

	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	t.Cleanup(tr.Stop)

	port := rand.Intn(32768) + 32768
	createReuseServer := func(port int) *UDPServer {
		return &UDPServer{
			network: udpnet,
			lc: &net.ListenConfig{
				Control: func(network, address string, c syscall.RawConn) error {
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
			onMessage: func(buf []byte, n int) []byte {
				return genPayload(serverMessageSize)
			},
		}
	}

	doneChan := make(chan struct{})
	t.Cleanup(func() { close(doneChan) })
	s1 := createReuseServer(port)
	s2 := createReuseServer(port)
	err = s1.Run(doneChan, clientMessageSize)
	require.NoError(t, err)

	err = s2.Run(doneChan, clientMessageSize)
	require.NoError(t, err)

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
	t.Logf("port: %d", port)
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

func TestDNSStatsWithNAT(t *testing.T) {
	state := testutil.IptablesSave(t)
	// Setup a NAT rule to translate 2.2.2.2 to 8.8.8.8 and issue a DNS request to 2.2.2.2
	cmds := []string{"iptables -t nat -A OUTPUT -d 2.2.2.2 -j DNAT --to-destination 8.8.8.8"}
	testutil.RunCommands(t, cmds, true)

	t.Cleanup(func() {
		testutil.IptablesRestore(t, state)
	})

	testDNSStats(t, "golang.org", 1, 0, 0, "2.2.2.2")
}

func iptablesWrapper(t *testing.T, f func()) {
	iptables, err := exec.LookPath("iptables")
	assert.Nil(t, err)

	// Init iptables rule to simulate packet loss
	rule := "INPUT --source 127.0.0.1 -j DROP"
	create := strings.Fields(fmt.Sprintf("-I %s", rule))

	state := testutil.IptablesSave(t)
	createCmd := exec.Command(iptables, create...)
	err = createCmd.Start()
	assert.Nil(t, err)
	err = createCmd.Wait()
	assert.Nil(t, err)

	defer func() {
		testutil.IptablesRestore(t, state)
	}()

	f()
}

func setupDNAT(t *testing.T) {
	if _, err := exec.LookPath("conntrack"); err != nil {
		t.Errorf("conntrack not found in PATH: %s", err)
		return
	}

	state := testutil.IptablesSave(t)
	t.Cleanup(func() { teardownDNAT(t, state) })
	// Using dummy1 instead of dummy0 (https://serverfault.com/a/841723)
	cmds := []string{
		"ip link add dummy1 type dummy",
		"ip address add 1.1.1.1 broadcast + dev dummy1",
		"ip link set dummy1 up",
		"iptables -t nat -A OUTPUT --dest 2.2.2.2 -j DNAT --to-destination 1.1.1.1",
	}
	testutil.RunCommands(t, cmds, false)
}

func teardownDNAT(t *testing.T, state []byte) {
	if len(state) > 0 {
		testutil.IptablesRestore(t, state)
	}

	cmds := []string{
		// tear down the testing interface, and iptables rule
		"ip link del dummy1",
		// clear out the conntrack table
		"conntrack -F",
	}
	testutil.RunCommands(t, cmds, true)
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

func TestSendfileRegression(t *testing.T) {
	// Start tracer
	tr, err := NewTracer(testConfig())
	require.NoError(t, err)
	defer tr.Stop()

	// Create temporary file
	tmpfile, err := os.CreateTemp("", "sendfile_source")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())
	n, err := tmpfile.Write(genPayload(clientMessageSize))
	require.NoError(t, err)
	require.Equal(t, clientMessageSize, n)
	_, err = tmpfile.Seek(0, 0)
	require.NoError(t, err)

	// Start TCP server
	var rcvd int64
	server := NewTCPServer(func(c net.Conn) {
		rcvd, _ = io.Copy(io.Discard, c)
		c.Close()
	})
	doneChan := make(chan struct{})
	err = server.Run(doneChan)
	require.NoError(t, err)
	defer close(doneChan)

	// Connect to TCP server
	c, err := net.DialTimeout("tcp", server.address, time.Second)
	require.NoError(t, err)

	// Warm up state
	_ = getConnections(t, tr)

	// Grab file size
	stat, err := tmpfile.Stat()
	require.NoError(t, err)
	fsize := int(stat.Size())

	// Send file contents via SENDFILE(2)
	n, err = sendFile(t, c, tmpfile, nil, fsize)
	require.NoError(t, err)
	require.Equal(t, fsize, n)

	// Verify that our TCP server received the contents of the file
	c.Close()
	require.Eventually(t, func() bool {
		return int64(clientMessageSize) == rcvd
	}, 3*time.Second, 500*time.Millisecond, "TCP server didn't receive data")

	// Finally, retrieve connection and assert that the sendfile was accounted for
	var conn *network.ConnectionStats
	require.Eventually(t, func() bool {
		conns := getConnections(t, tr)
		t.Logf("%+v", conns.Conns)
		var ok bool
		conn, ok = findConnection(c.LocalAddr(), c.RemoteAddr(), conns)
		return ok && conn.Monotonic.SentBytes > 0
	}, 3*time.Second, 500*time.Millisecond, "couldn't find connection used by sendfile(2)")

	assert.Equalf(t, int64(clientMessageSize), int64(conn.Monotonic.SentBytes), "sendfile data wasn't properly traced")
}

func TestSendfileError(t *testing.T) {
	tr, err := NewTracer(testConfig())
	require.NoError(t, err)
	t.Cleanup(tr.Stop)

	tmpfile, err := os.CreateTemp("", "sendfile_source")
	require.NoError(t, err)
	t.Cleanup(func() { os.Remove(tmpfile.Name()) })

	n, err := tmpfile.Write(genPayload(clientMessageSize))
	require.NoError(t, err)
	require.Equal(t, clientMessageSize, n)
	_, err = tmpfile.Seek(0, 0)
	require.NoError(t, err)

	server := NewTCPServer(func(c net.Conn) {
		_, _ = io.Copy(io.Discard, c)
		c.Close()
	})
	doneChan := make(chan struct{})
	err = server.Run(doneChan)
	require.NoError(t, err)
	t.Cleanup(func() { close(doneChan) })

	c, err := net.DialTimeout("tcp", server.address, time.Second)
	require.NoError(t, err)

	// Warm up state
	_ = getConnections(t, tr)

	// Send file contents via SENDFILE(2)
	offset := int64(math.MaxInt64 - 1)
	_, err = sendFile(t, c, tmpfile, &offset, 10)
	require.Error(t, err)

	c.Close()

	var conn *network.ConnectionStats
	require.Eventually(t, func() bool {
		conns := getConnections(t, tr)
		var ok bool
		conn, ok = findConnection(c.LocalAddr(), c.RemoteAddr(), conns)
		return ok
	}, 3*time.Second, 500*time.Millisecond, "couldn't find connection used by sendfile(2)")

	assert.Equalf(t, int64(0), int64(conn.Monotonic.SentBytes), "sendfile data wasn't properly traced")
}

func sendFile(t *testing.T, c net.Conn, f *os.File, offset *int64, count int) (int, error) {
	// Send payload using SENDFILE(2) syscall
	rawConn, err := c.(*net.TCPConn).SyscallConn()
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

func TestShortWrite(t *testing.T) {
	tr, err := NewTracer(testConfig())
	require.NoError(t, err)
	t.Cleanup(tr.Stop)

	read := make(chan struct{})
	server := NewTCPServer(func(c net.Conn) {
		// set recv buffer to 0 and don't read
		// to fill up tcp window
		err := c.(*net.TCPConn).SetReadBuffer(0)
		require.NoError(t, err)
		<-read
		c.Close()
	})
	doneChan := make(chan struct{})
	err = server.Run(doneChan)
	require.NoError(t, err)
	t.Cleanup(func() {
		close(read)
		close(doneChan)
	})

	s, err := unix.Socket(syscall.AF_INET, syscall.SOCK_STREAM|syscall.SOCK_NONBLOCK, 0)
	require.NoError(t, err)
	defer syscall.Close(s)

	err = unix.SetsockoptInt(s, syscall.SOL_SOCKET, syscall.SO_SNDBUF, 5000)
	require.NoError(t, err)

	sndBufSize, err := unix.GetsockoptInt(s, syscall.SOL_SOCKET, syscall.SO_SNDBUF)
	require.NoError(t, err)
	require.GreaterOrEqual(t, sndBufSize, 5000)

	var sa unix.SockaddrInet4
	host, portStr, err := net.SplitHostPort(server.address)
	require.NoError(t, err)
	copy(sa.Addr[:], net.ParseIP(host).To4())
	port, err := strconv.ParseInt(portStr, 10, 32)
	require.NoError(t, err)
	sa.Port = int(port)

	err = unix.Connect(s, &sa)
	if syscall.EINPROGRESS != err {
		require.NoError(t, err)
	}

	var wfd unix.FdSet
	wfd.Zero()
	wfd.Set(s)
	tv := unix.NsecToTimeval(int64((5 * time.Second).Nanoseconds()))
	nfds, err := unix.Select(s+1, nil, &wfd, nil, &tv)
	require.NoError(t, err)
	require.Equal(t, 1, nfds)

	var written int
	done := false
	var sent uint64
	toSend := sndBufSize / 2
	for i := 0; i < 100; i++ {
		written, err = unix.Write(s, genPayload(toSend))
		require.Greater(t, written, 0)
		require.NoError(t, err)
		sent += uint64(written)
		t.Logf("sent: %v", sent)
		if written < toSend {
			done = true
			break
		}
	}

	require.True(t, done)

	f := os.NewFile(uintptr(s), "")
	defer f.Close()
	c, err := net.FileConn(f)
	require.NoError(t, err)

	var conn *network.ConnectionStats
	require.Eventually(t, func() bool {
		conns := getConnections(t, tr)
		var ok bool
		conn, ok = findConnection(c.LocalAddr(), c.RemoteAddr(), conns)
		return ok
	}, 3*time.Second, 500*time.Millisecond, "couldn't find connection used by short write")

	assert.Equal(t, sent, conn.Monotonic.SentBytes)
}

func TestKprobeAttachWithKprobeEvents(t *testing.T) {
	cfg := config.New()
	cfg.AttachKprobesWithKprobeEventsABI = true

	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	cmd := []string{"curl", "-k", "-o/dev/null", "facebook.com"}
	exec.Command(cmd[0], cmd[1:]...).Run()

	stats := ddebpf.GetProbeStats()
	require.NotNil(t, stats)

	p_tcp_sendmsg, ok := stats["p_tcp_sendmsg_hits"]
	require.True(t, ok)
	fmt.Printf("p_tcp_sendmsg_hits = %d\n", p_tcp_sendmsg)

	assert.Greater(t, p_tcp_sendmsg, int64(0))
}

func TestBlockingReadCounts(t *testing.T) {
	tr, err := NewTracer(testConfig())
	require.NoError(t, err)
	t.Cleanup(tr.Stop)

	_ = getConnections(t, tr)

	server := NewTCPServer(func(c net.Conn) {
		c.Write([]byte("foo"))
		time.Sleep(time.Second)
		c.Write([]byte("foo"))
	})

	done := make(chan struct{})
	server.Run(done)
	defer func() { close(done) }()

	c, err := net.DialTimeout("tcp", server.address, 5*time.Second)
	require.NoError(t, err)
	defer c.Close()

	f, err := c.(*net.TCPConn).File()
	require.NoError(t, err)

	buf := make([]byte, 6)
	n, _, err := syscall.Recvfrom(int(f.Fd()), buf, syscall.MSG_WAITALL)
	require.NoError(t, err)

	assert.Equal(t, 6, n)

	var conn *network.ConnectionStats
	require.Eventually(t, func() bool {
		var found bool
		conn, found = findConnection(c.(*net.TCPConn).LocalAddr(), c.(*net.TCPConn).RemoteAddr(), getConnections(t, tr))
		return found
	}, 3*time.Second, 500*time.Millisecond)

	assert.Equal(t, uint64(n), conn.Monotonic.RecvBytes)
}

func TestTCPDirectionWithPreexistingConnection(t *testing.T) {
	wg := sync.WaitGroup{}

	// setup server to listen on a port
	server := NewTCPServer(func(c net.Conn) {
		t.Logf("received connection from %s", c.RemoteAddr())
		_, _ = bufio.NewReader(c).ReadBytes('\n')
		c.Close()
		wg.Done()
	})
	doneChan := make(chan struct{})
	server.Run(doneChan)
	defer close(doneChan)
	t.Logf("server address: %s", server.address)

	// create an initial client connection to the server
	c, err := net.DialTimeout("tcp", server.address, 5*time.Second)
	require.NoError(t, err)
	defer c.Close()

	// start tracer so it dumps port bindings
	cfg := testConfig()
	// delay from gateway lookup timeout can cause test failure
	cfg.EnableGatewayLookup = false
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	initTracerState(t, tr)

	// open and close another client connection to force port binding delete
	wg.Add(1)
	c2, err := net.DialTimeout("tcp", server.address, 5*time.Second)
	require.NoError(t, err)
	_, err = c2.Write([]byte("conn2\n"))
	require.NoError(t, err)
	c2.Close()

	wg.Wait()

	wg.Add(1)
	// write some data so tracer determines direction of this connection
	_, err = c.Write([]byte("original\n"))
	require.NoError(t, err)

	wg.Wait()

	// the original connection should still be incoming for the server
	conns := getConnections(t, tr)
	origConn := searchConnections(conns, func(cs network.ConnectionStats) bool {
		return fmt.Sprintf("%s:%d", cs.Source, cs.SPort) == server.address &&
			fmt.Sprintf("%s:%d", cs.Dest, cs.DPort) == c.LocalAddr().String()
	})
	require.Len(t, origConn, 1)
	require.Equal(t, network.INCOMING, origConn[0].Direction, "original server<->client connection should have incoming direction")
}

func TestPreexistingConnectionDirection(t *testing.T) {
	// Start the client and server before we enable the system probe to test that the tracer picks
	// up the pre-existing connection
	doneChan := make(chan struct{})

	server := NewTCPServer(func(c net.Conn) {
		r := bufio.NewReader(c)
		_, _ = r.ReadBytes(byte('\n'))
		_, _ = c.Write(genPayload(serverMessageSize))
		_ = c.Close()
	})
	err := server.Run(doneChan)
	require.NoError(t, err)

	c, err := net.DialTimeout("tcp", server.address, 50*time.Millisecond)
	require.NoError(t, err)
	defer func() { _ = c.Close() }()

	if _, err = c.Write(genPayload(clientMessageSize)); err != nil {
		t.Fatal(err)
	}

	// Enable BPF-based system probe
	tr, err := NewTracer(testConfig())
	require.NoError(t, err)
	defer tr.Stop()

	// Write more data so that the tracer will notice the connection
	_, err = c.Write(genPayload(clientMessageSize))
	require.NoError(t, err)

	r := bufio.NewReader(c)
	_, _ = r.ReadBytes(byte('\n'))

	initTracerState(t, tr)
	connections := getConnections(t, tr)

	conn, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	require.True(t, ok)
	m := conn.Monotonic
	assert.Equal(t, clientMessageSize, int(m.SentBytes))
	assert.Equal(t, serverMessageSize, int(m.RecvBytes))
	assert.Equal(t, 0, int(m.Retransmits))
	assert.Equal(t, os.Getpid(), int(conn.Pid))
	assert.Equal(t, addrPort(server.address), int(conn.DPort))
	assert.Equal(t, network.OUTGOING, conn.Direction)
	assert.True(t, conn.IntraHost)

	doneChan <- struct{}{}
}
