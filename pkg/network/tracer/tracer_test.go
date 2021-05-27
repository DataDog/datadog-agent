// +build linux_bpf

package tracer

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	nethttp "net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/config/sysctl"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/http"
	"github.com/DataDog/datadog-agent/pkg/network/netlink"
	"github.com/DataDog/datadog-agent/pkg/network/testutil"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf"
	"github.com/cihub/seelog"
	"github.com/golang/mock/gomock"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netns"
)

var (
	clientMessageSize = 2 << 8
	serverMessageSize = 2 << 14
	payloadSizesTCP   = []int{2 << 5, 2 << 8, 2 << 10, 2 << 12, 2 << 14, 2 << 15}
	payloadSizesUDP   = []int{2 << 5, 2 << 8, 2 << 12, 2 << 14}
)

// runtimeCompilationEnvVar forces use of the runtime compiler for ebpf functionality
const runtimeCompilationEnvVar = "DD_TESTS_RUNTIME_COMPILED"

func TestMain(m *testing.M) {
	log.SetupLogger(seelog.Default, "trace")
	cfg := testConfig()
	if cfg.EnableRuntimeCompiler {
		fmt.Println("RUNTIME COMPILER ENABLED")
	}
	os.Exit(m.Run())
}

func TestGetStats(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	pre410Kernel := currKernelVersion < kernel.VersionCode(4, 1, 0)

	cfg := testConfig()
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	<-time.After(time.Second)

	expected := map[string][]string{
		"conntrack": {
			"state_size",
			"enobufs",
			"throttles",
			"sampling_pct",
			"read_errors",
			"msg_errors",
		},
		"state": {
			"stats_resets",
			"closed_conn_dropped",
			"conn_dropped",
			"time_sync_collisions",
			"dns_stats_dropped",
			"dns_pid_collisions",
		},
		"tracer": {
			"closed_conn_polling_lost",
			"closed_conn_polling_received",
			"conn_valid_skipped",
			"expired_tcp_conns",
			"pid_collisions",
		},
		"ebpf": {
			"tcp_sent_miscounts",
			"missed_tcp_close",
		},
		"dns": {
			"added",
			"decoding_errors",
			"errors",
			"expired",
			"ips",
			"lookups",
			"oversized",
			"packets_captured",
			"packets_dropped",
			"packets_processed",
			"queries",
			"resolved",
			"socket_polls",
			"successes",
			"timestamp_micro_secs",
			"truncated_packets",
		},
		"kprobes": nil,
	}

	actual, _ := tr.GetStats()

	for section, entries := range expected {
		if section == "dns" && pre410Kernel {
			// DNS stats not supported on <4.1.0
			continue
		}

		require.Contains(t, actual, section, "missing section from telemetry map: %s", section)
		for _, name := range entries {
			assert.Contains(t, actual[section], name, "%s actual is missing %s", section, name)
		}
	}
}

func TestTCPSendAndReceive(t *testing.T) {
	// Enable BPF-based system probe
	tr, err := NewTracer(testConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	// Create TCP Server which, for every line, sends back a message with size=serverMessageSize
	server := NewTCPServer(func(c net.Conn) {
		r := bufio.NewReader(c)
		for {
			_, err := r.ReadBytes(byte('\n'))
			c.Write(genPayload(serverMessageSize))
			if err != nil { // indicates that EOF has been reached,
				break
			}
		}
		c.Close()
	})
	doneChan := make(chan struct{})
	err = server.Run(doneChan)
	require.NoError(t, err)

	c, err := net.DialTimeout("tcp", server.address, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Connect to server 10 times
	wg := sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Write clientMessageSize to server, and read response
			if _, err = c.Write(genPayload(clientMessageSize)); err != nil {
				t.Fatal(err)
			}

			r := bufio.NewReader(c)
			r.ReadBytes(byte('\n'))
		}()
	}

	wg.Wait()

	// Iterate through active connections until we find connection created above, and confirm send + recv counts
	connections := getConnections(t, tr)

	conn, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	require.True(t, ok)
	assert.Equal(t, 10*clientMessageSize, int(conn.MonotonicSentBytes))
	assert.Equal(t, 10*serverMessageSize, int(conn.MonotonicRecvBytes))
	assert.Equal(t, 0, int(conn.MonotonicRetransmits))
	assert.Equal(t, os.Getpid(), int(conn.Pid))
	assert.Equal(t, addrPort(server.address), int(conn.DPort))
	assert.Equal(t, network.OUTGOING, conn.Direction)
	assert.True(t, conn.IntraHost)

	doneChan <- struct{}{}
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

	connections := getConnections(t, tr)

	conn, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	require.True(t, ok)
	assert.Equal(t, clientMessageSize, int(conn.MonotonicSentBytes))
	assert.Equal(t, serverMessageSize, int(conn.MonotonicRecvBytes))
	assert.Equal(t, 0, int(conn.MonotonicRetransmits))
	assert.Equal(t, os.Getpid(), int(conn.Pid))
	assert.Equal(t, addrPort(server.address), int(conn.DPort))
	assert.Equal(t, network.OUTGOING, conn.Direction)
	assert.True(t, conn.IntraHost)

	doneChan <- struct{}{}
}

func TestDNATIntraHostIntegration(t *testing.T) {
	t.SkipNow()
	setupDNAT(t)
	defer teardownDNAT(t)

	tr, err := NewTracer(testConfig())
	require.NoError(t, err)
	defer tr.Stop()

	conns := getConnections(t, tr).Conns

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

	conns = getConnections(t, tr).Conns
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

	// Assert the TCP map does not contain first connection because of the clean up
	tcpMp, err := tr.getMap(probes.TcpStatsMap)
	require.NoError(t, err)

	key, err := connTupleFromConn(c, 0, 0)
	require.NoError(t, err)
	stats := new(TCPStats)
	err = tcpMp.Lookup(unsafe.Pointer(key), unsafe.Pointer(stats))
	if !assert.True(t, errors.Is(err, ebpf.ErrKeyNotExist)) {
		t.Logf("tcp_stats map entries:\n")
		ek := &ConnTuple{}
		sv := new(TCPStats)
		entries := tcpMp.IterateFrom(unsafe.Pointer(&ConnTuple{}))
		for entries.Next(unsafe.Pointer(ek), unsafe.Pointer(sv)) {
			t.Logf("%s => %+v\n", ek, sv)
		}
		require.NoError(t, entries.Err())
	}

	conn, ok := findConnection(c2.LocalAddr(), c2.RemoteAddr(), connections)
	require.True(t, ok)
	assert.Equal(t, clientMessageSize, int(conn.MonotonicSentBytes))
	assert.Equal(t, 0, int(conn.MonotonicRecvBytes))
	assert.Equal(t, 0, int(conn.MonotonicRetransmits))
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
	assert.Equal(t, 100*clientMessageSize, int(conn.MonotonicSentBytes))
	assert.True(t, int(conn.MonotonicRetransmits) > 0)
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
		io.Copy(ioutil.Discard, c)
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
	socketFile.Close()
	c.Close()

	// Fetch all connections matching source and target address
	allConnections := getConnections(t, tr)
	conns := searchConnections(allConnections, byAddress(c.LocalAddr(), c.RemoteAddr()))
	require.Len(t, conns, numProcesses)

	totalSent := 0
	for _, c := range conns {
		totalSent += int(c.MonotonicSentBytes)
	}
	assert.Equal(t, numProcesses*clientMessageSize, totalSent)

	// Since we can't reliably identify the PID associated to a retransmit, we have opted
	// to report the total number of retransmits for *one* of the connections sharing the
	// same socket
	connsWithRetransmits := 0
	for _, c := range conns {
		if c.MonotonicRetransmits > 0 {
			connsWithRetransmits++
		}
	}
	assert.Equal(t, 1, connsWithRetransmits)

	// Test if telemetry measuring PID collisions is correct
	// >= because there can be other connections going on during CI that increase pidCollisions
	assert.GreaterOrEqual(t, atomic.LoadInt64(&tr.pidCollisions), int64(numProcesses-1))
}

func TestTCPRTT(t *testing.T) {
	// Enable BPF-based system probe
	tr, err := NewTracer(testConfig())
	require.NoError(t, err)
	defer tr.Stop()

	// Create TCP Server that simply "drains" connection until receiving an EOF
	server := NewTCPServer(func(c net.Conn) {
		io.Copy(ioutil.Discard, c)
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

	// Obtain information from a TCP socket via GETSOCKOPT(2) system call.
	tcpInfo, err := tcpGetInfo(c)
	require.NoError(t, err)

	// Write something to socket to ensure connection is tracked
	// This will trigger the collection of TCP stats including RTT
	_, err = c.Write([]byte("foo"))
	require.NoError(t, err)

	// Fetch connection matching source and target address
	allConnections := getConnections(t, tr)
	conn, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), allConnections)
	require.True(t, ok)

	// Assert that values returned from syscall match ones generated by eBPF program
	assert.EqualValues(t, int(tcpInfo.Rtt), int(conn.RTT))
	assert.EqualValues(t, int(tcpInfo.Rttvar), int(conn.RTTVar))
}

type AddrPair struct {
	local  net.Addr
	remote net.Addr
}

func TestTCPShortlived(t *testing.T) {
	// Enable BPF-based system probe
	cfg := testConfig()
	cfg.TCPClosedTimeout = 10 * time.Millisecond
	tr, err := NewTracer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	// Simulate registering by calling get one time
	getConnections(t, tr)

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
	c, err := net.DialTimeout("tcp", server.address, 50*time.Millisecond)
	require.NoError(t, err)

	// Write clientMessageSize to server, and read response
	_, err = c.Write(genPayload(clientMessageSize))
	require.NoError(t, err)
	r := bufio.NewReader(c)
	r.ReadBytes(byte('\n'))

	// Explicitly close this TCP connection
	c.Close()

	var conn *network.ConnectionStats
	require.Eventually(t, func() bool {
		var ok bool
		conn, ok = findConnection(c.LocalAddr(), c.RemoteAddr(), getConnections(t, tr))
		return ok
	}, 3*time.Second, time.Second, "connection not found")

	assert.Equal(t, clientMessageSize, int(conn.MonotonicSentBytes))
	assert.Equal(t, serverMessageSize, int(conn.MonotonicRecvBytes))
	assert.Equal(t, 0, int(conn.MonotonicRetransmits))
	assert.Equal(t, os.Getpid(), int(conn.Pid))
	assert.Equal(t, addrPort(server.address), int(conn.DPort))
	assert.Equal(t, network.OUTGOING, conn.Direction)
	assert.True(t, conn.IntraHost)

	// Verify the short lived connection is accounting for both TCP_ESTABLISHED and TCP_CLOSED events
	assert.Equal(t, uint32(1), conn.MonotonicTCPEstablished)
	assert.Equal(t, uint32(1), conn.MonotonicTCPClosed)

	_, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), getConnections(t, tr))
	assert.False(t, ok)
}

func TestTCPOverIPv6(t *testing.T) {
	t.SkipNow()
	if !kernel.IsIPv6Enabled() {
		t.Skip("IPv6 not enabled on host")
	}

	config := testConfig()
	config.CollectIPv6Conns = true

	tr, err := NewTracer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	ln, err := net.Listen("tcp6", ":0")
	require.NoError(t, err)

	doneChan := make(chan struct{})
	go func(done chan struct{}) {
		<-done
		ln.Close()
	}(doneChan)

	// Create TCP Server which sends back serverMessageSize bytes
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			r := bufio.NewReader(c)
			r.ReadBytes(byte('\n'))
			c.Write(genPayload(serverMessageSize))
			c.Close()
		}
	}()

	// Connect to server
	c, err := net.DialTimeout("tcp6", ln.Addr().String(), 50*time.Millisecond)
	require.NoError(t, err)

	// Write clientMessageSize to server, and read response
	if _, err = c.Write(genPayload(clientMessageSize)); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(c)
	r.ReadBytes(byte('\n'))

	connections := getConnections(t, tr)

	conn, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	require.True(t, ok)
	assert.Equal(t, clientMessageSize, int(conn.MonotonicSentBytes))
	assert.Equal(t, serverMessageSize, int(conn.MonotonicRecvBytes))
	assert.Equal(t, 0, int(conn.MonotonicRetransmits))
	assert.Equal(t, os.Getpid(), int(conn.Pid))
	assert.Equal(t, ln.Addr().(*net.TCPAddr).Port, int(conn.DPort))
	assert.Equal(t, network.OUTGOING, conn.Direction)
	assert.True(t, conn.IntraHost)

	doneChan <- struct{}{}

}

func TestTCPCollectionDisabled(t *testing.T) {
	// Enable BPF-based system probe with TCP disabled
	config := testConfig()
	config.CollectTCPConns = false

	tr, err := NewTracer(config)
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
	c, err := net.DialTimeout("tcp", server.address, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	// Write clientMessageSize to server, and read response
	if _, err = c.Write(genPayload(clientMessageSize)); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(c)
	r.ReadBytes(byte('\n'))

	connections := getConnections(t, tr)

	// Confirm that we could not find connection created above
	_, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	require.False(t, ok)
}

func TestUDPSendAndReceive(t *testing.T) {
	// Enable BPF-based system probe
	cfg := testConfig()
	cfg.BPFDebug = true
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	server := NewUDPServerOnAddress("127.0.0.1:8001", func(buf []byte, n int) []byte {
		return genPayload(serverMessageSize)
	})

	doneChan := make(chan struct{})
	err = server.Run(doneChan, clientMessageSize)
	require.NoError(t, err)
	defer close(doneChan)

	// Connect to server
	c, err := net.DialTimeout("udp", server.address, 50*time.Millisecond)
	require.NoError(t, err)
	defer c.Close()

	// Write clientMessageSize to server, and read response
	_, err = c.Write(genPayload(clientMessageSize))
	require.NoError(t, err)

	_, err = c.Read(make([]byte, serverMessageSize))
	require.NoError(t, err)

	// Iterate through active connections until we find connection created above, and confirm send + recv counts
	connections := getConnections(t, tr)

	incoming, ok := findConnection(c.RemoteAddr(), c.LocalAddr(), connections)
	require.True(t, ok)
	require.Equal(t, network.INCOMING, incoming.Direction)

	outgoing, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	require.True(t, ok)
	require.Equal(t, network.OUTGOING, outgoing.Direction)

	require.Equal(t, clientMessageSize, int(outgoing.MonotonicSentBytes))
	require.Equal(t, serverMessageSize, int(outgoing.MonotonicRecvBytes))
	require.True(t, outgoing.IntraHost)

	// make sure the inverse values are seen for the other message
	require.Equal(t, serverMessageSize, int(incoming.MonotonicSentBytes))
	require.Equal(t, clientMessageSize, int(incoming.MonotonicRecvBytes))
	require.True(t, incoming.IntraHost)
}

func TestUDPPeekCount(t *testing.T) {
	config := testConfig()
	config.BPFDebug = true
	tr, err := NewTracer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "testdata/peek.py")
	err = cmd.Start()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	laddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	raddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:34568")
	require.NoError(t, err)

	c, err := net.DialUDP("udp", laddr, raddr)
	require.NoError(t, err)
	defer c.Close()

	msg := []byte("asdf")
	_, err = c.Write(msg)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	connections := getConnections(t, tr)

	incoming, ok := findConnection(c.RemoteAddr(), c.LocalAddr(), connections)
	require.True(t, ok)

	outgoing, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	require.True(t, ok)

	require.Equal(t, len(msg), int(outgoing.MonotonicSentBytes))
	require.Equal(t, 0, int(outgoing.MonotonicRecvBytes))
	require.True(t, outgoing.IntraHost)

	// make sure the inverse values are seen for the other message
	require.Equal(t, 0, int(incoming.MonotonicSentBytes))
	require.Equal(t, len(msg), int(incoming.MonotonicRecvBytes))
	require.True(t, incoming.IntraHost)
}
func TestUDPDisabled(t *testing.T) {
	// Enable BPF-based system probe with UDP disabled
	config := testConfig()
	config.CollectUDPConns = false

	tr, err := NewTracer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	// Create UDP Server which sends back serverMessageSize bytes
	server := NewUDPServer(func(b []byte, n int) []byte {
		return genPayload(serverMessageSize)
	})

	doneChan := make(chan struct{})
	err = server.Run(doneChan, clientMessageSize)
	require.NoError(t, err)
	defer close(doneChan)

	// Connect to server
	c, err := net.DialTimeout("udp", server.address, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Write clientMessageSize to server, and read response
	if _, err = c.Write(genPayload(clientMessageSize)); err != nil {
		t.Fatal(err)
	}

	c.Read(make([]byte, serverMessageSize))

	// Iterate through active connections until we find connection created above, and confirm send + recv counts
	connections := getConnections(t, tr)

	_, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	require.False(t, ok)
}

func TestLocalDNSCollectionDisabled(t *testing.T) {
	// Enable BPF-based system probe with DNS disabled (by default)
	config := testConfig()

	tr, err := NewTracer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	// Connect to local DNS
	addr, err := net.ResolveUDPAddr("udp", "localhost:53")
	assert.NoError(t, err)

	cn, err := net.DialUDP("udp", nil, addr)
	assert.NoError(t, err)
	defer cn.Close()

	// Write anything
	_, err = cn.Write([]byte("test"))
	assert.NoError(t, err)

	// Iterate through active connections making sure there are no local DNS calls
	for _, c := range getConnections(t, tr).Conns {
		assert.False(t, isLocalDNS(c))
	}
}

func TestLocalDNSCollectionEnabled(t *testing.T) {
	// Enable BPF-based system probe with DNS enabled
	config := testConfig()
	config.CollectLocalDNS = true
	config.CollectUDPConns = true

	tr, err := NewTracer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	// Connect to local DNS
	addr, err := net.ResolveUDPAddr("udp", "localhost:53")
	assert.NoError(t, err)

	cn, err := net.DialUDP("udp", nil, addr)
	assert.NoError(t, err)
	defer cn.Close()

	// Write anything
	_, err = cn.Write([]byte("test"))
	assert.NoError(t, err)

	found := false
	// Iterate through active connections making sure theres at least one connection
	for _, c := range getConnections(t, tr).Conns {
		found = found || isLocalDNS(c)
	}

	assert.True(t, found)
}

func isLocalDNS(c network.ConnectionStats) bool {
	return c.Source.String() == "127.0.0.1" && c.Dest.String() == "127.0.0.1" && c.DPort == 53
}

func TestShouldSkipExcludedConnection(t *testing.T) {
	// exclude connections from 127.0.0.1:80
	config := testConfig()
	// exclude source SSH connections to make this pass in VM
	config.ExcludedSourceConnections = map[string][]string{"127.0.0.1": {"80"}, "*": {"22"}}
	config.ExcludedDestinationConnections = map[string][]string{"127.0.0.1": {"tcp 80"}}
	tr, err := NewTracer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	// Connect to 127.0.0.1:80
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:80")
	assert.NoError(t, err)

	cn, err := net.DialUDP("udp", nil, addr)
	assert.NoError(t, err)
	defer cn.Close()

	// Write anything
	_, err = cn.Write([]byte("test"))
	assert.NoError(t, err)

	// Make sure we're not picking up 127.0.0.1:80
	for _, c := range getConnections(t, tr).Conns {
		assert.False(t, c.Source.String() == "127.0.0.1" && c.SPort == 80, "connection %s should be excluded", c)
		assert.False(t, c.Dest.String() == "127.0.0.1" && c.DPort == 80 && c.Type == network.TCP, "connection %s should be excluded", c)
	}

	// ensure one of the connections is UDP to 127.0.0.1:80
	assert.Condition(t, func() bool {
		for _, c := range getConnections(t, tr).Conns {
			if c.Dest.String() == "127.0.0.1" && c.DPort == 80 && c.Type == network.UDP {
				return true
			}
		}
		return false
	}, "Unable to find UDP connection to 127.0.0.1:80")
}

func TestIsExpired(t *testing.T) {
	// 10mn
	var timeout uint64 = 600000000000
	for _, tc := range []struct {
		stats      ConnStatsWithTimestamp
		latestTime uint64
		expected   bool
	}{
		{
			ConnStatsWithTimestamp{timestamp: 101},
			100,
			false,
		},
		{
			ConnStatsWithTimestamp{timestamp: 100},
			101,
			false,
		},
		{
			ConnStatsWithTimestamp{timestamp: 100},
			101 + timeout,
			true,
		},
	} {
		assert.Equal(t, tc.expected, tc.stats.isExpired(tc.latestTime, timeout))
	}
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
	assert.False(t, uint64(len(x)) == conn.MonotonicSentBytes)

	tel := tr.getEbpfTelemetry()
	assert.NotZero(t, tel["tcp_sent_miscounts"])
}

func TestSkipConnectionDNS(t *testing.T) {

	t.Run("CollectLocalDNS disabled", func(t *testing.T) {
		tr := &Tracer{config: &config.Config{CollectLocalDNS: false}}
		assert.True(t, tr.shouldSkipConnection(&network.ConnectionStats{
			Source: util.AddressFromString("10.0.0.1"),
			Dest:   util.AddressFromString("127.0.0.1"),
			SPort:  1000, DPort: 53,
		}))

		assert.False(t, tr.shouldSkipConnection(&network.ConnectionStats{
			Source: util.AddressFromString("10.0.0.1"),
			Dest:   util.AddressFromString("127.0.0.1"),
			SPort:  1000, DPort: 8080,
		}))

		assert.True(t, tr.shouldSkipConnection(&network.ConnectionStats{
			Source: util.AddressFromString("::3f::45"),
			Dest:   util.AddressFromString("::1"),
			SPort:  53, DPort: 1000,
		}))

		assert.True(t, tr.shouldSkipConnection(&network.ConnectionStats{
			Source: util.AddressFromString("::3f::45"),
			Dest:   util.AddressFromString("::1"),
			SPort:  53, DPort: 1000,
		}))
	})

	t.Run("CollectLocalDNS disabled", func(t *testing.T) {
		tr := &Tracer{config: &config.Config{CollectLocalDNS: true}}

		assert.False(t, tr.shouldSkipConnection(&network.ConnectionStats{
			Source: util.AddressFromString("10.0.0.1"),
			Dest:   util.AddressFromString("127.0.0.1"),
			SPort:  1000, DPort: 53,
		}))

		assert.False(t, tr.shouldSkipConnection(&network.ConnectionStats{
			Source: util.AddressFromString("10.0.0.1"),
			Dest:   util.AddressFromString("127.0.0.1"),
			SPort:  1000, DPort: 8080,
		}))

		assert.False(t, tr.shouldSkipConnection(&network.ConnectionStats{
			Source: util.AddressFromString("::3f::45"),
			Dest:   util.AddressFromString("::1"),
			SPort:  53, DPort: 1000,
		}))

		assert.False(t, tr.shouldSkipConnection(&network.ConnectionStats{
			Source: util.AddressFromString("::3f::45"),
			Dest:   util.AddressFromString("::1"),
			SPort:  53, DPort: 1000,
		}))

	})
}

func TestConnectionExpirationRegression(t *testing.T) {
	t.SkipNow()
	tr, err := NewTracer(testConfig())
	require.NoError(t, err)
	defer tr.Stop()

	// Create TCP Server that simply "drains" connection until receiving an EOF
	connClosed := make(chan struct{})
	server := NewTCPServer(func(c net.Conn) {
		io.Copy(ioutil.Discard, c)
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
	assert.Equal(t, uint64(len(payload)), connectionStats.LastSentBytes)

	// This emulates the race condition, a `tcp_close` followed by a call to `Tracer.removeConnections()`
	// It's unfortunate we're relying here on private methods, but there isn't much we can do to avoid that.
	c.Close()
	<-connClosed
	time.Sleep(100 * time.Millisecond)
	removeConnection(t, tr, connectionStats)

	// Since no bytes were send or received after we obtained the connectionStats, we should have 0 LastBytesSent
	allConnections = getConnections(t, tr)
	connectionStats, ok = findConnection(c.LocalAddr(), c.RemoteAddr(), allConnections)
	require.True(t, ok)
	assert.Equal(t, uint64(0), connectionStats.LastSentBytes)

	// Finally, this connection should have been expired from the state
	allConnections = getConnections(t, tr)
	_, ok = findConnection(c.LocalAddr(), c.RemoteAddr(), allConnections)
	require.False(t, ok)
}

func removeConnection(t *testing.T, tr *Tracer, c *network.ConnectionStats) {
	mp, err := tr.getMap(probes.ConnMap)
	require.NoError(t, err)

	tcpMp, err := tr.getMap(probes.TcpStatsMap)
	require.NoError(t, err)

	tuple := []*ConnTuple{
		{
			pid:      _Ctype_uint(c.Pid),
			saddr_l:  _Ctype_ulonglong(nativeEndian.Uint32(c.Source.Bytes())),
			daddr_l:  _Ctype_ulonglong(nativeEndian.Uint32(c.Dest.Bytes())),
			sport:    _Ctype_ushort(c.SPort),
			dport:    _Ctype_ushort(c.DPort),
			netns:    _Ctype_uint(c.NetNS),
			metadata: 1, // TCP/IPv4
		},
	}

	tr.removeEntries(mp, tcpMp, tuple)
}

func byAddress(l, r net.Addr) func(c network.ConnectionStats) bool {
	return func(c network.ConnectionStats) bool {
		return addrMatches(l, c.Source.String(), c.SPort) && addrMatches(r, c.Dest.String(), c.DPort)
	}
}

func findConnection(l, r net.Addr, c *network.Connections) (*network.ConnectionStats, bool) {
	if result := searchConnections(c, byAddress(l, r)); len(result) > 0 {
		return &result[0], true
	}

	return nil, false
}

func searchConnections(c *network.Connections, predicate func(network.ConnectionStats) bool) []network.ConnectionStats {
	var results []network.ConnectionStats
	for _, conn := range c.Conns {
		if predicate(conn) {
			results = append(results, conn)
		}
	}
	return results
}

func addrMatches(addr net.Addr, host string, port uint16) bool {
	addrURL := url.URL{Scheme: addr.Network(), Host: addr.String()}

	return addrURL.Hostname() == host && addrURL.Port() == strconv.Itoa(int(port))
}

func runBenchtests(b *testing.B, payloads []int, prefix string, f func(p int) func(*testing.B)) {
	for _, p := range payloads {
		name := strings.TrimSpace(strings.Join([]string{prefix, strconv.Itoa(p), "bytes"}, " "))
		b.Run(name, f(p))
	}
}

func BenchmarkUDPEcho(b *testing.B) {
	runBenchtests(b, payloadSizesUDP, "", benchEchoUDP)

	// Enable BPF-based system probe
	t, err := NewTracer(testConfig())
	if err != nil {
		b.Fatal(err)
	}
	defer t.Stop()

	runBenchtests(b, payloadSizesUDP, "eBPF", benchEchoUDP)
}

func benchEchoUDP(size int) func(b *testing.B) {
	payload := genPayload(size)
	echoOnMessage := func(b []byte, n int) []byte {
		resp := make([]byte, len(b))
		copy(resp, b)
		return resp
	}

	return func(b *testing.B) {
		end := make(chan struct{})
		server := NewUDPServer(echoOnMessage)
		err := server.Run(end, size)
		require.NoError(b, err)
		defer close(end)

		c, err := net.DialTimeout("udp", server.address, 50*time.Millisecond)
		if err != nil {
			b.Fatal(err)
		}
		defer c.Close()
		r := bufio.NewReader(c)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			c.Write(payload)
			buf := make([]byte, size)
			n, err := r.Read(buf)

			if err != nil || n != len(payload) || !bytes.Equal(payload, buf) {
				b.Fatalf("Sizes: %d, %d. Equal: %v. Error: %s", len(buf), len(payload), bytes.Equal(payload, buf), err)
			}
		}
		b.StopTimer()
	}
}

func BenchmarkTCPEcho(b *testing.B) {
	runBenchtests(b, payloadSizesTCP, "", benchEchoTCP)

	// Enable BPF-based system probe
	t, err := NewTracer(testConfig())
	if err != nil {
		b.Fatal(err)
	}
	defer t.Stop()

	runBenchtests(b, payloadSizesTCP, "eBPF", benchEchoTCP)
}

func BenchmarkTCPSend(b *testing.B) {
	runBenchtests(b, payloadSizesTCP, "", benchSendTCP)

	// Enable BPF-based system probe
	t, err := NewTracer(testConfig())
	if err != nil {
		b.Fatal(err)
	}
	defer t.Stop()

	runBenchtests(b, payloadSizesTCP, "eBPF", benchSendTCP)
}

func benchEchoTCP(size int) func(b *testing.B) {
	payload := genPayload(size)
	echoOnMessage := func(c net.Conn) {
		r := bufio.NewReader(c)
		for {
			buf, err := r.ReadBytes(byte('\n'))
			if err == io.EOF {
				c.Close()
				return
			}
			c.Write(buf)
		}
	}

	return func(b *testing.B) {
		end := make(chan struct{})
		server := NewTCPServer(echoOnMessage)
		err := server.Run(end)
		require.NoError(b, err)
		defer close(end)

		c, err := net.DialTimeout("tcp", server.address, 50*time.Millisecond)
		if err != nil {
			b.Fatal(err)
		}
		defer c.Close()
		r := bufio.NewReader(c)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			c.Write(payload)
			buf, err := r.ReadBytes(byte('\n'))

			if err != nil || len(buf) != len(payload) || !bytes.Equal(payload, buf) {
				b.Fatalf("Sizes: %d, %d. Equal: %v. Error: %s", len(buf), len(payload), bytes.Equal(payload, buf), err)
			}
		}
		b.StopTimer()
	}
}

func benchSendTCP(size int) func(b *testing.B) {
	payload := genPayload(size)
	dropOnMessage := func(c net.Conn) {
		r := bufio.NewReader(c)
		for { // Drop all payloads received
			_, err := r.Discard(r.Buffered() + 1)
			if err == io.EOF {
				c.Close()
				return
			}
		}
	}

	return func(b *testing.B) {
		end := make(chan struct{})
		server := NewTCPServer(dropOnMessage)
		err := server.Run(end)
		require.NoError(b, err)
		defer close(end)

		c, err := net.DialTimeout("tcp", server.address, 50*time.Millisecond)
		if err != nil {
			b.Fatal(err)
		}
		defer c.Close()

		b.ResetTimer()
		for i := 0; i < b.N; i++ { // Send-heavy workload
			_, err := c.Write(payload)
			if err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
	}
}

type TCPServer struct {
	address   string
	onMessage func(c net.Conn)
}

func NewTCPServer(onMessage func(c net.Conn)) *TCPServer {
	return NewTCPServerOnAddress("127.0.0.1:0", onMessage)
}

func NewTCPServerOnAddress(addr string, onMessage func(c net.Conn)) *TCPServer {
	return &TCPServer{
		address:   addr,
		onMessage: onMessage,
	}
}

func (s *TCPServer) Run(done chan struct{}) error {
	ln, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}
	s.address = ln.Addr().String()

	go func() {
		<-done
		ln.Close()
	}()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.onMessage(conn)
		}
	}()

	return nil
}

type UDPServer struct {
	address   string
	onMessage func(b []byte, n int) []byte
}

func NewUDPServer(onMessage func(b []byte, n int) []byte) *UDPServer {
	return NewUDPServerOnAddress("127.0.0.1:0", onMessage)
}

func NewUDPServerOnAddress(addr string, onMessage func(b []byte, n int) []byte) *UDPServer {
	return &UDPServer{
		address:   addr,
		onMessage: onMessage,
	}
}

func (s *UDPServer) Run(done chan struct{}, payloadSize int) error {
	ln, err := net.ListenPacket("udp", s.address)
	if err != nil {
		return err
	}

	s.address = ln.LocalAddr().String()

	go func() {
		buf := make([]byte, payloadSize)
		running := true
		for running {
			select {
			case <-done:
				running = false
			default:
				ln.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
				n, addr, err := ln.ReadFrom(buf)
				if err != nil {
					break
				}
				_, err = ln.WriteTo(s.onMessage(buf, n), addr)
				if err != nil {
					fmt.Println(err)
					break
				}
			}
		}

		ln.Close()
	}()

	return nil
}

var letterBytes = []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func genPayload(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		if i == n-1 {
			b[i] = '\n'
		} else {
			b[i] = letterBytes[rand.Intn(len(letterBytes))]
		}
	}
	return b
}

func iptablesWrapper(t *testing.T, f func()) {
	iptables, err := exec.LookPath("iptables")
	assert.Nil(t, err)

	// Init iptables rule to simulate packet loss
	rule := "INPUT --source 127.0.0.1 -j DROP"
	create := strings.Fields(fmt.Sprintf("-I %s", rule))
	remove := strings.Fields(fmt.Sprintf("-D %s", rule))

	createCmd := exec.Command(iptables, create...)
	err = createCmd.Start()
	assert.Nil(t, err)
	err = createCmd.Wait()
	assert.Nil(t, err)

	defer func() {
		// Remove the iptable rule
		removeCmd := exec.Command(iptables, remove...)
		err = removeCmd.Start()
		assert.Nil(t, err)
		err = removeCmd.Wait()
		assert.Nil(t, err)
	}()

	f()
}

func addrPort(addr string) int {
	p, _ := strconv.Atoi(strings.Split(addr, ":")[1])
	return p
}

func getConnections(t *testing.T, tr *Tracer) *network.Connections {
	// Iterate through active connections until we find connection created above, and confirm send + recv counts
	connections, err := tr.GetActiveConnections("1")
	if err != nil {
		t.Fatal(err)
	}

	return connections
}

const (
	validDNSServer = "8.8.8.8"
)

func testDNSStats(t *testing.T, domain string, success int, failure int, timeout int, serverIP string) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	pre410Kernel := currKernelVersion < kernel.VersionCode(4, 1, 0)
	if pre410Kernel {
		t.Skip("DNS feature not available on pre 4.1.0 kernels")
		return
	}

	config := testConfig()
	config.CollectDNSStats = true
	config.DNSTimeout = 1 * time.Second
	tr, err := NewTracer(config)
	require.NoError(t, err)
	defer tr.Stop()

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

	if err != nil && timeout == 0 {
		t.Fatalf("Failed to get dns response %s\n", err.Error())
	}

	// Allow the DNS reply to be processed in the snooper
	time.Sleep(time.Millisecond * 500)

	// Iterate through active connections until we find connection created above, and confirm send + recv counts
	connections := getConnections(t, tr)
	conn, ok := findConnection(dnsClientAddr, dnsServerAddr, connections)
	require.True(t, ok)

	assert.Equal(t, queryMsg.Len(), int(conn.MonotonicSentBytes))
	assert.Equal(t, os.Getpid(), int(conn.Pid))
	assert.Equal(t, dnsServerAddr.Port, int(conn.DPort))

	// DNS Stats
	assert.Equal(t, uint32(success), conn.DNSSuccessfulResponses)
	assert.Equal(t, uint32(failure), conn.DNSFailedResponses)
	assert.Equal(t, uint32(timeout), conn.DNSTimeouts)
}

func TestDNSStatsForValidDomain(t *testing.T) {
	testDNSStats(t, "golang.org", 1, 0, 0, validDNSServer)
}

func TestDNSStatsForInvalidDomain(t *testing.T) {
	testDNSStats(t, "abcdedfg", 0, 1, 0, validDNSServer)
}

func TestDNSStatsForTimeout(t *testing.T) {
	testDNSStats(t, "golang.org", 0, 0, 1, "1.2.3.4")
}

func TestConntrackExpiration(t *testing.T) {
	setupDNAT(t)
	defer teardownDNAT(t)

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
		io.Copy(ioutil.Discard, c)
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

func TestTCPEstablished(t *testing.T) {
	// Ensure closed connections are flushed as soon as possible
	cfg := testConfig()
	cfg.TCPClosedTimeout = 500 * time.Millisecond

	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	// Warm-up state
	getConnections(t, tr)

	server := NewTCPServer(func(c net.Conn) {
		io.Copy(ioutil.Discard, c)
		c.Close()
	})
	doneChan := make(chan struct{})
	err = server.Run(doneChan)
	require.NoError(t, err)
	defer close(doneChan)

	c, err := net.DialTimeout("tcp", server.address, 50*time.Millisecond)
	require.NoError(t, err)

	laddr, raddr := c.LocalAddr(), c.RemoteAddr()
	c.Write([]byte("hello"))

	connections := getConnections(t, tr)
	conn, ok := findConnection(laddr, raddr, connections)

	require.True(t, ok)
	assert.Equal(t, uint32(1), conn.LastTCPEstablished)
	assert.Equal(t, uint32(0), conn.LastTCPClosed)

	c.Close()
	// Wait for the connection to be sent from the perf buffer
	time.Sleep(cfg.TCPClosedTimeout)

	connections = getConnections(t, tr)
	conn, ok = findConnection(laddr, raddr, connections)
	require.True(t, ok)
	assert.Equal(t, uint32(0), conn.LastTCPEstablished)
	assert.Equal(t, uint32(1), conn.LastTCPClosed)
}

func TestTCPEstablishedPreExistingConn(t *testing.T) {
	server := NewTCPServer(func(c net.Conn) {
		io.Copy(ioutil.Discard, c)
		c.Close()
	})
	doneChan := make(chan struct{})
	err := server.Run(doneChan)
	require.NoError(t, err)
	defer close(doneChan)

	c, err := net.DialTimeout("tcp", server.address, 50*time.Millisecond)
	require.NoError(t, err)
	laddr, raddr := c.LocalAddr(), c.RemoteAddr()

	// Ensure closed connections are flushed as soon as possible
	cfg := testConfig()
	cfg.TCPClosedTimeout = 500 * time.Millisecond

	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	// Warm-up state
	getConnections(t, tr)

	c.Write([]byte("hello"))
	c.Close()
	// Wait for the connection to be sent from the perf buffer
	time.Sleep(cfg.TCPClosedTimeout)
	connections := getConnections(t, tr)
	conn, ok := findConnection(laddr, raddr, connections)

	require.True(t, ok)
	assert.Equal(t, uint32(0), conn.MonotonicTCPEstablished)
	assert.Equal(t, uint32(1), conn.MonotonicTCPClosed)
}

func TestUnconnectedUDPSendIPv4(t *testing.T) {
	cfg := testConfig()
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	remotePort := rand.Int()%5000 + 15000
	remoteAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: remotePort}
	// Use ListenUDP instead of DialUDP to create a "connectionless" UDP connection
	conn, err := net.ListenUDP("udp", nil)
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
	assert.Equal(t, bytesSent, int(outgoing[0].MonotonicSentBytes))
}

func TestConnectedUDPSendIPv6(t *testing.T) {
	if !kernel.IsIPv6Enabled() {
		t.Skip("IPv6 not enabled on host")
	}

	cfg := testConfig()
	cfg.CollectIPv6Conns = true
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	remotePort := rand.Int()%5000 + 15000
	remoteAddr := &net.UDPAddr{IP: net.IPv6loopback, Port: remotePort}
	conn, err := net.DialUDP("udp6", nil, remoteAddr)
	require.NoError(t, err)
	defer conn.Close()
	message := []byte("payload")
	bytesSent, err := conn.Write(message)
	require.NoError(t, err)

	connections := getConnections(t, tr)
	outgoing := searchConnections(connections, func(cs network.ConnectionStats) bool {
		return cs.DPort == uint16(remotePort)
	})

	require.Len(t, outgoing, 1)
	assert.Equal(t, remoteAddr.IP.String(), outgoing[0].Dest.String())
	assert.Equal(t, bytesSent, int(outgoing[0].MonotonicSentBytes))
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
	assert.Equal(t, bytesSent, int(outgoing[0].MonotonicSentBytes))
}

func TestConnectionClobber(t *testing.T) {
	tr, err := NewTracer(testConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	// Create TCP Server which, for every line, sends back a message with size=serverMessageSize
	var serverConns []net.Conn
	srvRecvBuf := make([]byte, 4)
	server := NewTCPServer(func(c net.Conn) {
		serverConns = append(serverConns, c)
		_, _ = io.ReadFull(c, srvRecvBuf)
		_, _ = c.Write(srvRecvBuf)
	})
	doneChan := make(chan struct{})
	server.Run(doneChan)
	defer close(doneChan)

	// we only need 1/4 since both send and recv sides will be registered
	sendCount := cap(tr.buffer)/4 + 1
	sendAndRecv := func() []net.Conn {
		connsCh := make(chan net.Conn, sendCount)
		var conns []net.Conn
		go func() {
			for c := range connsCh {
				conns = append(conns, c)
			}
		}()

		workers := sync.WaitGroup{}
		for i := 0; i < sendCount; i++ {
			workers.Add(1)
			go func() {
				defer workers.Done()

				c, err := net.DialTimeout("tcp", server.address, 5*time.Second)
				require.NoError(t, err)
				connsCh <- c

				buf := make([]byte, 4)
				_, err = c.Write(buf)
				require.NoError(t, err)

				_, err = io.ReadFull(c, buf[:0])
				require.NoError(t, err)
			}()
		}

		workers.Wait()
		close(connsCh)

		return conns
	}

	conns := sendAndRecv()

	// wait for tracer to pick up all connections
	//
	// there is not good way do this other than a sleep since we
	// can't call getConnections in a require.Eventually call
	// to the get the number of connections as that could
	// affect the tr.buffer length
	time.Sleep(2 * time.Second)

	preCap := cap(tr.buffer)
	connections := getConnections(t, tr)
	src := connections.Conns[0].SPort
	dst := connections.Conns[0].DPort
	t.Logf("got %d connections", len(connections.Conns))
	// ensure we didn't grow or shrink the buffer
	require.Equal(t, preCap, cap(tr.buffer))

	// send second batch so that underlying array gets clobbered
	conns = append(conns, sendAndRecv()...)
	defer func() {
		for _, c := range append(conns, serverConns...) {
			c.Close()
		}
	}()

	time.Sleep(2 * time.Second)

	t.Logf("got %d connections", len(getConnections(t, tr).Conns))
	require.Equal(t, src, connections.Conns[0].SPort, "source port should not change")
	require.Equal(t, dst, connections.Conns[0].DPort, "dest port should not change")
	require.Equal(t, preCap, cap(tr.buffer))
}

func TestTCPDirection(t *testing.T) {
	cfg := testConfig()
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	// Warm-up tracer state
	_ = getConnections(t, tr)

	// Start an HTTP server on localhost:8080
	serverAddr := "127.0.0.1:8080"
	srv := &nethttp.Server{
		Addr: serverAddr,
		Handler: nethttp.HandlerFunc(func(w nethttp.ResponseWriter, req *nethttp.Request) {
			t.Logf("received http request from %s", req.RemoteAddr)
			io.Copy(ioutil.Discard, req.Body)
			w.WriteHeader(200)
		}),
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
	}
	srv.SetKeepAlivesEnabled(false)
	go func() {
		_ = srv.ListenAndServe()
	}()
	defer srv.Shutdown(context.Background())

	// Allow the HTTP server time to get set up
	time.Sleep(time.Millisecond * 500)

	// Send a HTTP request to the test server
	client := new(nethttp.Client)
	resp, err := client.Get("http://" + serverAddr + "/test")
	require.NoError(t, err)
	resp.Body.Close()

	// Iterate through active connections until we find connection created above
	var outgoingConns []network.ConnectionStats
	var incomingConns []network.ConnectionStats
	require.Eventuallyf(t, func() bool {
		conns := getConnections(t, tr)
		if len(outgoingConns) == 0 {
			outgoingConns = searchConnections(conns, func(cs network.ConnectionStats) bool {
				return fmt.Sprintf("%s:%d", cs.Dest, cs.DPort) == serverAddr
			})
		}
		if len(incomingConns) == 0 {
			incomingConns = searchConnections(conns, func(cs network.ConnectionStats) bool {
				return fmt.Sprintf("%s:%d", cs.Source, cs.SPort) == serverAddr
			})
		}

		return len(outgoingConns) == 1 && len(incomingConns) == 1
	}, 3*time.Second, 10*time.Millisecond, "couldn't find incoming and outgoing http connections matching: %s", serverAddr)

	// Verify connection directions
	conn := outgoingConns[0]
	assert.Equal(t, conn.Direction, network.OUTGOING, "connection direction must be outgoing: %s", conn)
	conn = incomingConns[0]
	assert.Equal(t, conn.Direction, network.INCOMING, "connection direction must be incoming: %s", conn)
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
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	// Warm-up tracer state
	_ = getConnections(t, tr)

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

func TestEnableHTTPMonitoring(t *testing.T) {
	cfg := testConfig()
	cfg.EnableHTTPMonitoring = true
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()
}

func TestHTTPStats(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	pre410Kernel := currKernelVersion < kernel.VersionCode(4, 1, 0)
	if pre410Kernel {
		t.Skip("HTTP monitoring feature not available on pre 4.1.0 kernels")
		return
	}

	cfg := testConfig()
	cfg.EnableHTTPMonitoring = true
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	// Warm-up tracer state
	_ = getConnections(t, tr)

	// Start an HTTP server on localhost:8080
	serverAddr := "127.0.0.1:8080"
	srv := &nethttp.Server{
		Addr: serverAddr,
		Handler: nethttp.HandlerFunc(func(w nethttp.ResponseWriter, req *nethttp.Request) {
			io.Copy(ioutil.Discard, req.Body)
			w.WriteHeader(200)
		}),
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
	}
	srv.SetKeepAlivesEnabled(false)
	go func() {
		_ = srv.ListenAndServe()
	}()
	defer srv.Shutdown(context.Background())

	// Allow the HTTP server time to get set up
	time.Sleep(time.Millisecond * 500)

	// Send a series of HTTP requests to the test server
	client := new(nethttp.Client)
	resp, err := client.Get("http://" + serverAddr + "/test")
	require.NoError(t, err)
	resp.Body.Close()

	// Iterate through active connections until we find connection created above
	var httpReqStats http.RequestStats
	require.Eventuallyf(t, func() bool {
		payload, err := tr.GetActiveConnections("1")
		if err != nil {
			t.Fatal(err)
		}

		for key, stats := range payload.HTTP {
			if key.Path == "/test" {
				httpReqStats = stats
				return true
			}
		}

		return false
	}, 3*time.Second, 10*time.Millisecond, "couldn't find http connection matching: %s", serverAddr)

	// Verify HTTP stats
	assert.Equal(t, 0, httpReqStats[0].Count, "100s") // number of requests with response status 100
	assert.Equal(t, 1, httpReqStats[1].Count, "200s") // 200
	assert.Equal(t, 0, httpReqStats[2].Count, "300s") // 300
	assert.Equal(t, 0, httpReqStats[3].Count, "400s") // 400
	assert.Equal(t, 0, httpReqStats[4].Count, "500s") // 500
}

func TestRuntimeCompilerEnvironmentVar(t *testing.T) {
	cfg := testConfig()
	enabled := os.Getenv(runtimeCompilationEnvVar) != ""
	assert.Equal(t, enabled, cfg.EnableRuntimeCompiler)
	assert.NotEqual(t, enabled, cfg.AllowPrecompiledFallback)
}

func setupDNAT(t *testing.T) {
	if _, err := exec.LookPath("conntrack"); err != nil {
		t.Errorf("conntrack not found in PATH: %s", err)
		return
	}

	// Using dummy1 instead of dummy0 (https://serverfault.com/a/841723)
	cmds := []string{
		"ip link add dummy1 type dummy",
		"ip address add 1.1.1.1 broadcast + dev dummy1",
		"ip link set dummy1 up",
		"iptables -t nat -A OUTPUT --dest 2.2.2.2 -j DNAT --to-destination 1.1.1.1",
	}
	testutil.RunCommands(t, cmds, false)
}

func teardownDNAT(t *testing.T) {
	cmds := []string{
		// tear down the testing interface, and iptables rule
		"ip link del dummy1",
		"iptables -t nat -D OUTPUT -d 2.2.2.2 -j DNAT --to-destination 1.1.1.1",
		// clear out the conntrack table
		"conntrack -F",
	}
	testutil.RunCommands(t, cmds, true)
}

func testConfig() *config.Config {
	cfg := config.New()
	if os.Getenv(runtimeCompilationEnvVar) != "" {
		cfg.EnableRuntimeCompiler = true
		cfg.AllowPrecompiledFallback = false
	}
	return cfg
}

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

func TestGatewayLookupNotEnabled(t *testing.T) {
	t.Run("gateway lookup not enabled", func(t *testing.T) {
		cfg := testConfig()
		tr, err := NewTracer(cfg)
		require.NoError(t, err)
		require.NotNil(t, tr)
		defer tr.Stop()
		require.Nil(t, tr.gwLookup)
	})

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

	matches := ipRouteGetOut.FindSubmatch(out)
	require.Len(t, matches, 2, string(out))
	dev := string(matches[1])
	ifi, err := net.InterfaceByName(dev)
	require.NoError(t, err)
	return ifi
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

	cfg := testConfig()
	cfg.BPFDebug = true
	cfg.EnableGatewayLookup = true
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	require.NotNil(t, tr)
	defer tr.Stop()

	require.NotNil(t, tr.gwLookup)

	ifi := ipRouteGet(t, "", "8.8.8.8", nil)
	ifs, err := net.Interfaces()
	require.NoError(t, err)
	tr.gwLookup.subnetForHwAddrFunc = func(hwAddr net.HardwareAddr) (network.Subnet, error) {
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

	require.NotNil(t, conn.Via)
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
	cfg.BPFDebug = true
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
	cfg.BPFDebug = true
	cfg.EnableGatewayLookup = true
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	require.NotNil(t, tr)
	defer tr.Stop()

	require.NotNil(t, tr.gwLookup)
	// setup two network namespaces
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
	defer func() {
		testutil.RunCommands(t, []string{
			"iptables -D FORWARD -o br0 -j ACCEPT",
			"iptables -D FORWARD -i br0 -j ACCEPT",
			"iptables -D POSTROUTING -t nat -s 2.2.2.0/24 ! -d 2.2.2.0/24 -j MASQUERADE",
			"ip link del veth1",
			"ip link del veth3",
			"ip link del br0",
			"ip netns del test1",
			"ip netns del test2",
		}, true)
	}()
	testutil.RunCommands(t, cmds, false)

	ifs, err := net.Interfaces()
	tr.gwLookup.subnetForHwAddrFunc = func(hwAddr net.HardwareAddr) (network.Subnet, error) {
		for _, i := range ifs {
			if hwAddr.String() == i.HardwareAddr.String() {
				return network.Subnet{Alias: fmt.Sprintf("subnet-%s", i.Name)}, nil
			}
		}

		return network.Subnet{Alias: "subnet"}, nil
	}
	tr.gwLookup.purge()

	test1Ns, err := netns.GetFromName("test1")
	require.NoError(t, err)
	defer test1Ns.Close()

	// run tcp server in test1 net namespace
	done := make(chan struct{})
	var server *TCPServer
	err = util.WithNS("/proc", test1Ns, func() error {
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
		test2Ns, err := netns.GetFromName("test2")
		require.NoError(t, err)
		defer test2Ns.Close()

		var c net.Conn
		err = util.WithNS("/proc", test2Ns, func() error {
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
		util.WithNS("/proc", test2Ns, func() error {
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
	cfg.BPFDebug = true
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	require.NotNil(t, tr)
	defer tr.Stop()

	// register test as client
	getConnections(t, tr)

	server := NewUDPServer(func(b []byte, n int) []byte {
		return genPayload(serverMessageSize)
	})

	done := make(chan struct{})
	server.Run(done, clientMessageSize)
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
		return ok && conn.MonotonicSentBytes > 0 && conn.MonotonicRecvBytes > 0
	}, 3*time.Second, 500*time.Millisecond, "could not find udp connection")

	// verify the connection is marked as assured
	connMp, err := tr.getMap(probes.ConnMap)
	require.NoError(t, err)
	defer connMp.Close()
	key, err := connTupleFromConn(c, conn.Pid, conn.NetNS)
	stats := &ConnStatsWithTimestamp{}
	err = connMp.Lookup(unsafe.Pointer(key), unsafe.Pointer(stats))
	require.NoError(t, err)
	require.True(t, stats.isAssured())
}

func TestConnectionNotAssured(t *testing.T) {

	cfg := testConfig()
	cfg.BPFDebug = true
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	require.NotNil(t, tr)
	defer tr.Stop()

	// register test as client
	getConnections(t, tr)

	server := NewUDPServer(func(b []byte, n int) []byte {
		return nil
	})

	done := make(chan struct{})
	server.Run(done, clientMessageSize)
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
		return ok && conn.MonotonicSentBytes > 0 && conn.MonotonicRecvBytes == 0
	}, 3*time.Second, 500*time.Millisecond, "could not find udp connection")

	// verify the connection is marked as not assured
	connMp, err := tr.getMap(probes.ConnMap)
	require.NoError(t, err)
	defer connMp.Close()
	key, err := connTupleFromConn(c, conn.Pid, conn.NetNS)
	stats := &ConnStatsWithTimestamp{}
	err = connMp.Lookup(unsafe.Pointer(key), unsafe.Pointer(stats))
	require.NoError(t, err)
	require.False(t, stats.isAssured())
}

func TestSelfConnect(t *testing.T) {
	// Enable BPF-based system probe
	cfg := testConfig()
	cfg.BPFDebug = true
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

	// forked child should have exited, and only the parent should remain
	require.Eventually(t, func() bool {
		conns := searchConnections(getConnections(t, tr), func(cs network.ConnectionStats) bool {
			return cs.SPort == uint16(port) && cs.DPort == uint16(port) && cs.Source.IsLoopback() && cs.Dest.IsLoopback()
		})

		t.Logf("connections: %v", conns)
		return len(conns) == 1
	}, 5*time.Second, time.Second, "could not find expected number of tcp connections, expected: 1")
}

func TestNewConntracker(t *testing.T) {
	ctrl := gomock.NewController(t)

	cfg := testConfig()

	mockCreator := func(_ *config.Config) (netlink.Conntracker, error) {
		return netlink.NewMockConntracker(ctrl), nil
	}

	errCreator := func(_ *config.Config) (netlink.Conntracker, error) {
		return nil, assert.AnError
	}

	mockConntracker := netlink.NewMockConntracker(ctrl)
	noopConntracker := netlink.NewNoOpConntracker()

	tests := []struct {
		conntrackEnabled  bool
		ignoreInitFailure bool
		creator           func(*config.Config) (netlink.Conntracker, error)

		conntracker netlink.Conntracker
		err         error
	}{
		{false, false, mockCreator, noopConntracker, nil},
		{true, true, mockCreator, mockConntracker, nil},
		{true, true, errCreator, noopConntracker, nil},
		{true, false, mockCreator, mockConntracker, nil},
		{true, false, errCreator, nil, assert.AnError},
	}

	for _, te := range tests {
		cfg.EnableConntrack = te.conntrackEnabled
		cfg.IgnoreConntrackInitFailure = te.ignoreInitFailure
		c, err := newConntracker(cfg, te.creator)
		if te.conntracker != nil {
			require.IsType(t, te.conntracker, c)
		} else {
			require.Nil(t, c)
		}

		if te.err != nil {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
	}
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
