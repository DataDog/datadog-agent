// +build linux_bpf

package ebpf

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
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
	tr, err := NewTracer(NewDefaultConfig())
	require.NoError(t, err)
	defer tr.Stop()

	<-time.After(time.Second)

	expected := map[string][]string{
		"conntrack": {"NoopConntracker"},
		"state": {
			"UnorderedConns",
			"ConnDropped",
			"ClosedConnDropped",
			"StatsResets",
			"TimeSyncCollisions",
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
		},
		"dns": {
			"Lookups",
			"Resolved",
			"Ips",
		},
		"kprobes": {
			"PtcpCleanupRbufHits",
			"PtcpCleanupRbufMisses",
			"PtcpCloseHits",
			"PtcpCloseMisses",
			"PtcpRetransmitSkbHits",
			"PtcpRetransmitSkbMisses",
			"PtcpSendmsgHits",
			"PtcpSendmsgMisses",
			"PtcpVConnectHits",
			"PtcpVConnectMisses",
			"PtcpVDestroySockHits",
			"PtcpVDestroySockMisses",
			"PudpRecvmsgHits",
			"PudpRecvmsgMisses",
			"PudpSendmsgHits",
			"PudpSendmsgMisses",
			"RInetCskAcceptHits",
			"RInetCskAcceptMisses",
			"RTcpSendmsgHits",
			"RTcpSendmsgMisses",
			"RTcpVConnectHits",
			"RTcpVConnectMisses",
			"RUdpRecvmsgHits",
			"RUdpRecvmsgMisses",
			"RinetCskAcceptHits",
			"RinetCskAcceptMisses",
			"RtcpSendmsgHits",
			"RtcpSendmsgMisses",
			"RtcpVConnectHits",
			"RtcpVConnectMisses",
			"RudpRecvmsgHits",
			"RudpRecvmsgMisses",
		},
	}

	for _, et := range expvarTypes {
		expvar := map[string]float64{}
		require.NoError(t, json.Unmarshal([]byte(expvarEndpoints[et].String()), &expvar))
		assert.Len(t, expvar, len(expected[et]))
		for _, name := range expected[et] {
			assert.Contains(t, expvar, name)
		}
	}
}

func TestSnakeToCamel(t *testing.T) {
	for test, exp := range map[string]string{
		"closed_conn_dropped":              "ClosedConnDropped",
		"closed_conn_polling_lost":         "ClosedConnPollingLost",
		"Conntrack_short_Term_Buffer_size": "ConntrackShortTermBufferSize",
	} {
		assert.Equal(t, exp, snakeToCapInitialCamel(test))
	}
}

func TestTCPSendAndReceive(t *testing.T) {
	// Enable BPF-based system probe
	tr, err := NewTracer(NewDefaultConfig())
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
	server.Run(doneChan)

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
	assert.Equal(t, LOCAL, conn.Direction)

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
	server.Run(doneChan)

	c, err := net.DialTimeout("tcp", server.address, 50*time.Millisecond)
	require.NoError(t, err)
	defer func() { _ = c.Close() }()

	if _, err = c.Write(genPayload(clientMessageSize)); err != nil {
		t.Fatal(err)
	}

	// Enable BPF-based system probe
	tr, err := NewTracer(NewDefaultConfig())
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
	assert.Equal(t, LOCAL, conn.Direction)

	doneChan <- struct{}{}
}

func TestTCPRemoveEntries(t *testing.T) {
	config := NewDefaultConfig()
	config.TCPConnTimeout = 100 * time.Millisecond
	tr, err := NewTracer(config)

	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	// Create a dummy TCP Server
	server := NewTCPServer(func(c net.Conn) {
		c.Close()
	})
	doneChan := make(chan struct{})
	server.Run(doneChan)

	// Connect to server
	c, err := net.DialTimeout("tcp", server.address, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	// Write a message
	if _, err = c.Write(genPayload(clientMessageSize)); err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}

	// Send a messages
	if _, err = c2.Write(genPayload(clientMessageSize)); err != nil {
		t.Fatal(err)
	}
	defer c2.Close()

	// Retrieve the list of connections
	connections := getConnections(t, tr)

	// Make sure the first connection got cleaned up
	_, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	assert.False(t, ok)

	// Assert the TCP map is empty because of the clean up
	key, nextKey, stats := &ConnTuple{}, &ConnTuple{}, &ConnStatsWithTimestamp{}
	tcpMp, err := tr.getMap(tcpStatsMap)
	assert.Nil(t, err)
	// This should return false and an error
	hasNext, err := tr.m.LookupNextElement(tcpMp, unsafe.Pointer(key), unsafe.Pointer(nextKey), unsafe.Pointer(stats))
	assert.False(t, hasNext)
	assert.NotNil(t, err)

	conn, ok := findConnection(c2.LocalAddr(), c2.RemoteAddr(), connections)
	require.True(t, ok)
	assert.Equal(t, clientMessageSize, int(conn.MonotonicSentBytes))
	assert.Equal(t, 0, int(conn.MonotonicRecvBytes))
	assert.Equal(t, 0, int(conn.MonotonicRetransmits))
	assert.Equal(t, os.Getpid(), int(conn.Pid))
	assert.Equal(t, addrPort(server.address), int(conn.DPort))

	doneChan <- struct{}{}
}

func TestTCPRetransmit(t *testing.T) {
	// Enable BPF-based system probe
	tr, err := NewTracer(NewDefaultConfig())
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
	server.Run(doneChan)

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

	doneChan <- struct{}{}
}

func TestTCPRetransmitSharedSocket(t *testing.T) {
	// Enable BPF-based system probe
	tr, err := NewTracer(NewDefaultConfig())
	require.NoError(t, err)
	defer tr.Stop()

	// Create TCP Server that simply "drains" connection until receiving an EOF
	server := NewTCPServer(func(c net.Conn) {
		io.Copy(ioutil.Discard, c)
		c.Close()
	})
	doneChan := make(chan struct{})
	server.Run(doneChan)

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

	doneChan <- struct{}{}
}

func TestTCPRTT(t *testing.T) {
	// Enable BPF-based system probe
	tr, err := NewTracer(NewDefaultConfig())
	require.NoError(t, err)
	defer tr.Stop()

	// Create TCP Server that simply "drains" connection until receiving an EOF
	server := NewTCPServer(func(c net.Conn) {
		io.Copy(ioutil.Discard, c)
		c.Close()
	})
	doneChan := make(chan struct{})
	server.Run(doneChan)
	defer func() { doneChan <- struct{}{} }()

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
	assert.Equal(t, tcpInfo.Rtt, conn.RTT)
	assert.Equal(t, tcpInfo.Rttvar, conn.RTTVar)
}

func TestTCPShortlived(t *testing.T) {
	// Enable BPF-based system probe
	tr, err := NewTracer(NewDefaultConfig())
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
	server.Run(doneChan)

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
	time.Sleep(10 * time.Millisecond)

	connections := getConnections(t, tr)

	// Confirm that we can retrieve the shortlived connection
	conn, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	require.True(t, ok)
	assert.Equal(t, clientMessageSize, int(conn.MonotonicSentBytes))
	assert.Equal(t, serverMessageSize, int(conn.MonotonicRecvBytes))
	assert.Equal(t, 0, int(conn.MonotonicRetransmits))
	assert.Equal(t, os.Getpid(), int(conn.Pid))
	assert.Equal(t, addrPort(server.address), int(conn.DPort))
	assert.Equal(t, LOCAL, conn.Direction)

	// Confirm that the connection has been cleaned up since the last get
	connections = getConnections(t, tr)

	conn, ok = findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	assert.False(t, ok)

	doneChan <- struct{}{}
}

func TestTCPOverIPv6(t *testing.T) {
	config := NewDefaultConfig()
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
			if c, err := ln.Accept(); err != nil {
				return
			} else {
				r := bufio.NewReader(c)
				r.ReadBytes(byte('\n'))
				c.Write(genPayload(serverMessageSize))
				c.Close()
			}
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
	assert.Equal(t, LOCAL, conn.Direction)

	doneChan <- struct{}{}

}

func TestTCPCollectionDisabled(t *testing.T) {
	// Enable BPF-based system probe with TCP disabled
	config := NewDefaultConfig()
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
	server.Run(doneChan)

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

	doneChan <- struct{}{}
}

func TestUDPSendAndReceive(t *testing.T) {
	// Enable BPF-based system probe
	tr, err := NewTracer(NewDefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	// Create UDP Server which sends back serverMessageSize bytes
	server := NewUDPServer(func(b []byte, n int) []byte {
		return genPayload(serverMessageSize)
	})

	doneChan := make(chan struct{})
	server.Run(doneChan, clientMessageSize)

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

	conn, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	require.True(t, ok)
	assert.Equal(t, clientMessageSize, int(conn.MonotonicSentBytes))
	assert.Equal(t, serverMessageSize, int(conn.MonotonicRecvBytes))
	assert.Equal(t, os.Getpid(), int(conn.Pid))
	assert.Equal(t, addrPort(server.address), int(conn.DPort))
	assert.Equal(t, NONE, conn.Direction)

	doneChan <- struct{}{}
}

func TestUDPDisabled(t *testing.T) {
	// Enable BPF-based system probe with UDP disabled
	config := NewDefaultConfig()
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
	server.Run(doneChan, clientMessageSize)

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

	doneChan <- struct{}{}
}

func TestLocalDNSCollectionDisabled(t *testing.T) {
	// Enable BPF-based system probe with DNS disabled (by default)
	config := NewDefaultConfig()

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
	config := NewDefaultConfig()
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

func isLocalDNS(c ConnectionStats) bool {
	return c.Source.String() == "127.0.0.1" && c.Dest.String() == "127.0.0.1" && c.DPort == 53
}

func TestShouldSkipBlacklistedConnection(t *testing.T) {
	// exclude connections from 127.0.0.1:80
	config := NewDefaultConfig()
	config.ExcludedSourceConnections = map[string][]string{"127.0.0.1": {"80"}}
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
		assert.False(t, c.Source.String() == "127.0.0.1" && c.SPort == 80)
		assert.True(t, c.Dest.String() == "127.0.0.1" && c.DPort == 80)
	}
}

func TestTooSmallBPFMap(t *testing.T) {
	// Enable BPF-based system probe with BPF maps size = 1
	config := NewDefaultConfig()
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
	server.Run(doneChan)

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
	doneChan <- struct{}{}
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
	tr, err := NewTracer(NewDefaultConfig())
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
	server.Run(doneChan)

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
	assert.NoError(t, err)

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

func byAddress(l, r net.Addr) func(c ConnectionStats) bool {
	return func(c ConnectionStats) bool {
		return addrMatches(l, c.Source.String(), c.SPort) && addrMatches(r, c.Dest.String(), c.DPort)
	}
}

func findConnection(l, r net.Addr, c *Connections) (*ConnectionStats, bool) {
	if result := searchConnections(c, byAddress(l, r)); len(result) > 0 {
		return &result[0], true
	}

	return nil, false
}

func searchConnections(c *Connections, predicate func(ConnectionStats) bool) []ConnectionStats {
	var results []ConnectionStats
	for _, conn := range c.Conns {
		if predicate(conn) {
			results = append(results, conn)
		}
	}
	return results
}

func addrMatches(addr net.Addr, host string, port uint16) bool {
	addrUrl := url.URL{Scheme: addr.Network(), Host: addr.String()}

	return addrUrl.Hostname() == host && addrUrl.Port() == strconv.Itoa(int(port))
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
	t, err := NewTracer(NewDefaultConfig())
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
		server.Run(end, size)

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

		end <- struct{}{}
	}
}

func BenchmarkTCPEcho(b *testing.B) {
	runBenchtests(b, payloadSizesTCP, "", benchEchoTCP)

	// Enable BPF-based system probe
	t, err := NewTracer(NewDefaultConfig())
	if err != nil {
		b.Fatal(err)
	}
	defer t.Stop()

	runBenchtests(b, payloadSizesTCP, "eBPF", benchEchoTCP)
}

func BenchmarkTCPSend(b *testing.B) {
	runBenchtests(b, payloadSizesTCP, "", benchSendTCP)

	// Enable BPF-based system probe
	t, err := NewTracer(NewDefaultConfig())
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
		server.Run(end)

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

		end <- struct{}{}
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
		server.Run(end)

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

		end <- struct{}{}
	}
}

type TCPServer struct {
	address   string
	onMessage func(c net.Conn)
}

func NewTCPServer(onMessage func(c net.Conn)) *TCPServer {
	return &TCPServer{
		address:   "127.0.0.1:0",
		onMessage: onMessage,
	}
}

func (s *TCPServer) Run(done chan struct{}) {
	ln, err := net.Listen("tcp", s.address)
	if err != nil {
		fmt.Println(err)
		return
	}
	s.address = ln.Addr().String()

	go func() {
		<-done
		ln.Close()
	}()

	go func() {
		for {
			if conn, err := ln.Accept(); err != nil {
				return
			} else {
				s.onMessage(conn)
			}
		}
	}()
}

type UDPServer struct {
	address   string
	onMessage func(b []byte, n int) []byte
}

func NewUDPServer(onMessage func(b []byte, n int) []byte) *UDPServer {
	return &UDPServer{
		address:   "127.0.0.1:0",
		onMessage: onMessage,
	}
}

func (s *UDPServer) Run(done chan struct{}, payloadSize int) {
	ln, err := net.ListenPacket("udp", s.address)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	s.address = ln.LocalAddr().String()

	go func() {
		buf := make([]byte, payloadSize)
		for {
			select {
			case <-done:
				break
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
	create := strings.Fields(fmt.Sprintf("-A %s", rule))
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

func getConnections(t *testing.T, tr *Tracer) *Connections {
	// Iterate through active connections until we find connection created above, and confirm send + recv counts
	connections, err := tr.GetActiveConnections("1")
	if err != nil {
		t.Fatal(err)
	}

	return connections
}
