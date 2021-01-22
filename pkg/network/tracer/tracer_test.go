// +build linux_bpf

package tracer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/ebpf"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	clientMessageSize = 2 << 8
	serverMessageSize = 2 << 14
	payloadSizesTCP   = []int{2 << 5, 2 << 8, 2 << 10, 2 << 12, 2 << 14, 2 << 15}
	payloadSizesUDP   = []int{2 << 5, 2 << 8, 2 << 12, 2 << 14}
)

func TestTracerExpvar(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	pre410Kernel := currKernelVersion < kernel.VersionCode(4, 1, 0)

	cfg := config.NewDefaultConfig()
	// BPFDebug must be true for kretprobe/tcp_sendmsg to be included
	cfg.BPFDebug = true
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	<-time.After(time.Second)

	expected := map[string][]string{
		"conntrack": {
			"StateSize",
			"Enobufs",
			"Throttles",
			"SamplingPct",
			"ReadErrors",
			"MsgErrors",
		},
		"state": {
			"StatsResets",
			"UnorderedConns",
			"ClosedConnDropped",
			"ConnDropped",
			"TimeSyncCollisions",
			"DnsStatsDropped",
			"DnsPidCollisions",
		},
		"tracer": {
			"ClosedConnPollingLost",
			"ClosedConnPollingReceived",
			"ConnValidSkipped",
			"ExpiredTcpConns",
			"PidCollisions",
		},
		"ebpf": {
			"TcpSentMiscounts",
			"MissedTcpClose",
		},
		"dns": {
			"Added",
			"DecodingErrors",
			"Errors",
			"Expired",
			"Ips",
			"Lookups",
			"Oversized",
			"PacketsCaptured",
			"PacketsDropped",
			"PacketsProcessed",
			"Queries",
			"Resolved",
			"SocketPolls",
			"Successes",
			"TimestampMicroSecs",
			"TruncatedPackets",
		},
		"kprobes": {
			"PTcpCleanupRbufHits",
			"PTcpCleanupRbufMisses",
			"PTcpCloseHits",
			"PTcpCloseMisses",
			"PTcpRetransmitSkbHits",
			"PTcpRetransmitSkbMisses",
			"PTcpSendmsgHits",
			"PTcpSendmsgMisses",
			"PTcpSetStateHits",
			"PTcpSetStateMisses",
			"PTcpV4DestroySockHits",
			"PTcpV4DestroySockMisses",
			"PUdpDestroySockHits",
			"PUdpDestroySockMisses",
			"PUdpRecvmsgHits",
			"PUdpRecvmsgMisses",
			"PIpMakeSkbHits",
			"PIpMakeSkbMisses",
			"PInetBindHits",
			"PInetBindMisses",
			"PInet6BindHits",
			"PInet6BindMisses",
			"RInetCskAcceptHits",
			"RInetCskAcceptMisses",
			"RTcpCloseHits",
			"RTcpCloseMisses",
			"RUdpRecvmsgHits",
			"RUdpRecvmsgMisses",
			"RTcpSendmsgHits",
			"RTcpSendmsgMisses",
			"RInetBindHits",
			"RInetBindMisses",
			"RInet6BindHits",
			"RInet6BindMisses",
		},
	}

	archSpecificKprobes := [][]string{}

	for _, et := range expvarTypes {
		if et == "dns" && pre410Kernel {
			// DNS stats not supported on <4.1.0
			continue
		}

		expvar := map[string]float64{}
		require.NoError(t, json.Unmarshal([]byte(expvarEndpoints[et].String()), &expvar))
		for _, name := range expected[et] {
			assert.Contains(t, expvar, name, "%s actual is missing %s", et, name)
		}
		// check variants of arch-specific syscall kprobes
		if et == "kprobes" {
			for _, options := range archSpecificKprobes {
				inMap := false
				for _, opt := range options {
					_, inMap = expvar[opt]
					if inMap {
						break
					}
				}
				if !inMap {
					assert.Failf(t, "missing kprobe in expvar", "one of %v", options)
				}
			}
		}
	}
}

func TestTCPSendAndReceive(t *testing.T) {
	// Enable BPF-based system probe
	tr, err := NewTracer(config.NewDefaultConfig())
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
	tr, err := NewTracer(config.NewDefaultConfig())
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

	tr, err := NewTracer(config.NewDefaultConfig())
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
	config := config.NewDefaultConfig()
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

	key, err := connTupleFromConn(c, 0)
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
	tr, err := NewTracer(config.NewDefaultConfig())
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
	tr, err := NewTracer(config.NewDefaultConfig())
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
	assert.Equal(t, int64(numProcesses-1), atomic.LoadInt64(&tr.pidCollisions))
}

func TestTCPRTT(t *testing.T) {
	// Enable BPF-based system probe
	tr, err := NewTracer(config.NewDefaultConfig())
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
	cfg := config.NewDefaultConfig()
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
	if err != nil {
		t.Fatal(err)
	}

	// Write clientMessageSize to server, and read response
	if _, err = c.Write(genPayload(clientMessageSize)); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(c)
	r.ReadBytes(byte('\n'))

	// Explicitly close this TCP connection
	c.Close()

	// Wait for the message to be sent from the perf buffer
	time.Sleep(2 * cfg.TCPClosedTimeout)

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

	// Verify the short lived connection is accounting for both TCP_ESTABLISHED and TCP_CLOSED events
	assert.Equal(t, uint32(1), conn.MonotonicTCPEstablished)
	assert.Equal(t, uint32(1), conn.MonotonicTCPClosed)

	// Confirm that the connection has been cleaned up since the last get
	connections = getConnections(t, tr)

	conn, ok = findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	assert.False(t, ok)
}

func TestTCPOverIPv6(t *testing.T) {
	t.SkipNow()
	config := config.NewDefaultConfig()
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
	config := config.NewDefaultConfig()
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
	cfg := config.NewDefaultConfig()
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	cmd := exec.Command("../testdata/simulate_udp.sh")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("simulate_udp command output: %s", string(out))
	}

	defer func() {
		exec.Command("../testdata/teardown_simulate_udp.sh").Run()
	}()

	// Iterate through active connections until we find connection created above, and confirm send + recv counts
	connections := getConnections(t, tr)

	incoming := searchConnections(connections, func(cs network.ConnectionStats) bool {
		return cs.SPort == 8081
	})
	require.Len(t, incoming, 1)
	assert.Equal(t, network.INCOMING, incoming[0].Direction)

	outgoing := searchConnections(connections, func(cs network.ConnectionStats) bool {
		return cs.DPort == 8081
	})

	require.Len(t, outgoing, 1)
	assert.Equal(t, network.OUTGOING, outgoing[0].Direction)

	// these values come from simulate_udp.sh
	assert.Equal(t, 512, int(outgoing[0].MonotonicSentBytes))
	assert.Equal(t, 256, int(outgoing[0].MonotonicRecvBytes))
	assert.True(t, outgoing[0].IntraHost)

	// make sure the inverse values are seen for the other message
	assert.Equal(t, 256, int(incoming[0].MonotonicSentBytes))
	assert.Equal(t, 512, int(incoming[0].MonotonicRecvBytes))
	assert.True(t, incoming[0].IntraHost)
}

func TestUDPDisabled(t *testing.T) {
	// Enable BPF-based system probe with UDP disabled
	config := config.NewDefaultConfig()
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
	config := config.NewDefaultConfig()

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
	config := config.NewDefaultConfig()
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
	config := config.NewDefaultConfig()
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

func TestTooSmallBPFMap(t *testing.T) {
	// Enable BPF-based system probe with BPF maps size = 1
	config := config.NewDefaultConfig()
	config.MaxTrackedConnections = 1

	tr, err := NewTracer(config)
	require.NoError(t, err)
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

	// Connect to server two times
	// Write clientMessageSize to server
	c, err := net.DialTimeout("tcp", server.address, 50*time.Millisecond)
	require.NoError(t, err)
	defer c.Close()
	_, err = c.Write(genPayload(clientMessageSize))
	require.NoError(t, err)

	// Second time
	c2, err := net.DialTimeout("tcp", server.address, 50*time.Millisecond)
	require.NoError(t, err)
	defer c2.Close()
	_, err = c2.Write(genPayload(clientMessageSize))
	require.NoError(t, err)

	connections := getConnections(t, tr)
	// we should only have one connection returned
	assert.Len(t, connections.Conns, 1)
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
	tr, err := NewTracer(config.NewDefaultConfig())
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
	tr, err := NewTracer(config.NewDefaultConfig())
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
	t, err := NewTracer(config.NewDefaultConfig())
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
	t, err := NewTracer(config.NewDefaultConfig())
	if err != nil {
		b.Fatal(err)
	}
	defer t.Stop()

	runBenchtests(b, payloadSizesTCP, "eBPF", benchEchoTCP)
}

func BenchmarkTCPSend(b *testing.B) {
	runBenchtests(b, payloadSizesTCP, "", benchSendTCP)

	// Enable BPF-based system probe
	t, err := NewTracer(config.NewDefaultConfig())
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
			s.onMessage(conn)
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

	config := config.NewDefaultConfig()
	config.CollectDNSStats = true
	config.DNSTimeout = 1 * time.Second
	tr, err := NewTracer(config)
	require.NoError(t, err)
	defer tr.Stop()

	dnsServerAddr := &net.UDPAddr{IP: net.ParseIP(serverIP), Port: 53}

	queryMsg := new(dns.Msg)
	queryMsg.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	queryMsg.RecursionDesired = true

	// Get outbound IP
	dummyConn, err := net.Dial("udp", "8.8.8.8:80")
	require.NoError(t, err)
	dummyConn.Close()
	localAddr := dummyConn.LocalAddr().(*net.UDPAddr)

	dnsClientAddr := &net.UDPAddr{IP: localAddr.IP, Port: 7777}
	localAddrDialer := &net.Dialer{
		LocalAddr: dnsClientAddr,
	}

	dnsClient := dns.Client{Net: "udp", Dialer: localAddrDialer}
	_, _, err = dnsClient.Exchange(queryMsg, dnsServerAddr.String())

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

	tr, err := NewTracer(config.NewDefaultConfig())
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
	cfg := config.NewDefaultConfig()
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
	cfg := config.NewDefaultConfig()
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
	cfg := config.NewDefaultConfig()
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	remotePort := rand.Int()%5000 + 15000
	remoteAddr := &net.UDPAddr{IP: net.ParseIP("8.8.8.8"), Port: remotePort}
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
	cfg := config.NewDefaultConfig()
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

func TestConnectionClobber(t *testing.T) {
	tr, err := NewTracer(config.NewDefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	// Create TCP Server which, for every line, sends back a message with size=serverMessageSize
	srvRecvBuf := make([]byte, 4)
	server := NewTCPServer(func(c net.Conn) {
		_, _ = io.ReadFull(c, srvRecvBuf)
		_, _ = c.Write(srvRecvBuf)
	})
	doneChan := make(chan struct{})
	server.Run(doneChan)

	// we only need 1/4 since both send and recv sides will be registered
	sendCount := (cap(tr.buffer) / 4) + 1
	sendAndRecv := func(closeCh chan struct{}) *sync.WaitGroup {
		sendWg := sync.WaitGroup{}
		doneWg := sync.WaitGroup{}
		sendBuf := make([]byte, 4)
		recvBuf := make([]byte, 4)
		for i := 0; i < sendCount; i++ {
			senderNum := i
			sendWg.Add(1)
			doneWg.Add(1)
			go func() {
				defer doneWg.Done()

				c, err := net.DialTimeout("tcp", server.address, 5*time.Second)
				if err != nil {
					t.Logf("dial error %d: %s\n", senderNum, err)
					return
				}
				defer c.Close()

				if _, err = c.Write(sendBuf); err != nil {
					t.Fatal(err)
				}
				_, _ = io.ReadFull(c, recvBuf)
				sendWg.Done()
				<-closeCh
			}()
		}
		sendWg.Wait()
		return &doneWg
	}

	closeCh := make(chan struct{})
	dg := sendAndRecv(closeCh)

	preCap := cap(tr.buffer)
	firstConnections := getConnections(t, tr)
	// ensure we didn't grow or shrink the buffer
	require.Equal(t, preCap, cap(tr.buffer))
	src := firstConnections.Conns[0].SPort
	dst := firstConnections.Conns[0].DPort
	t.Logf("before src: %d dst: %d\n", src, dst)

	close(closeCh)
	dg.Wait()

	// send second batch so that underlying array gets clobbered
	closeCh = make(chan struct{})
	dg = sendAndRecv(closeCh)
	_ = getConnections(t, tr)
	require.Equal(t, preCap, cap(tr.buffer))

	t.Logf("after src: %d dst: %d\n", firstConnections.Conns[0].SPort, firstConnections.Conns[0].DPort)
	assert.EqualValues(t, int(src), int(firstConnections.Conns[0].SPort), "source port should not change")
	assert.EqualValues(t, int(dst), int(firstConnections.Conns[0].DPort), "dest port should not change")

	close(closeCh)
	dg.Wait()

	doneChan <- struct{}{}
}

func TestEnableHTTPMonitoring(t *testing.T) {
	cfg := config.NewDefaultConfig()
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

	cfg := config.NewDefaultConfig()
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
	go func() {
		_ = srv.ListenAndServe()
	}()
	defer srv.Shutdown(context.Background())

	// Allow the HTTP server time to get set up
	time.Sleep(time.Millisecond * 500)

	// Send a series of HTTP requests to the test server
	client := new(nethttp.Client)
	req, err := nethttp.NewRequest("GET", "http://"+serverAddr+"/test", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	resp.Body.Close()

	// Allow the HTTP transactions to be processed in the monitor
	time.Sleep(time.Second)

	// Iterate through active connections until we find connection created above
	conns := getConnections(t, tr)
	matchingConns := searchConnections(conns, func(cs network.ConnectionStats) bool {
		ip := cs.Dest.String()
		port := strconv.Itoa(int(cs.DPort))
		return ip+":"+port == serverAddr
	})
	require.Len(t, matchingConns, 1)

	// Verify HTTP stats
	conn := matchingConns[0]
	assert.Len(t, conn.HTTPStatsByPath, 1)

	httpReqStats, ok := conn.HTTPStatsByPath["/test"]
	assert.Equal(t, true, ok)
	assert.Equal(t, 0, httpReqStats.Count(0)) // number of requests with response status 100
	assert.Equal(t, 1, httpReqStats.Count(1)) // 200
	assert.Equal(t, 0, httpReqStats.Count(2)) // 300
	assert.Equal(t, 0, httpReqStats.Count(3)) // 400
	assert.Equal(t, 0, httpReqStats.Count(4)) // 500
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
	runCommands(t, cmds)
}

func teardownDNAT(t *testing.T) {
	cmds := []string{
		// tear down the testing interface, and iptables rule
		"ip link del dummy1",
		"iptables -t nat -D OUTPUT -d 2.2.2.2 -j DNAT --to-destination 1.1.1.1",
		// clear out the conntrack table
		"conntrack -F",
	}
	runCommands(t, cmds)
}

func runCommands(t *testing.T, cmds []string) {
	for _, c := range cmds {
		args := strings.Split(c, " ")
		c := exec.Command(args[0], args[1:]...)
		out, err := c.CombinedOutput()
		if err != nil {
			t.Errorf("%s returned %s: %s", c, err, out)
			return
		}
	}
}
