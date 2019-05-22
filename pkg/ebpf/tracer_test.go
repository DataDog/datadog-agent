// +build linux_bpf

package ebpf

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"os"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	clientMessageSize = 2 << 8
	serverMessageSize = 2 << 14
	payloadSizesTCP   = []int{2 << 5, 2 << 8, 2 << 10, 2 << 12, 2 << 14, 2 << 15}
	payloadSizesUDP   = []int{2 << 5, 2 << 8, 2 << 12, 2 << 14}
)

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

	// Iterate through active connections making sure theres at least one connection
	for _, c := range getConnections(t, tr).Conns {
		if isLocalDNS(c) {
			return
		}
	}

	// We shouldn't get here if the test passes successfully
	assert.Error(t, nil)
}

func isLocalDNS(c ConnectionStats) bool {
	return c.Source == "127.0.0.1" && c.Dest == "127.0.0.1" && c.DPort == 53
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

func findConnection(l, r net.Addr, c *Connections) (*ConnectionStats, bool) {
	for _, conn := range c.Conns {
		if addrMatches(l, conn.SourceAddr().String(), conn.SPort) && addrMatches(r, conn.DestAddr().String(), conn.DPort) {
			return &conn, true
		}
	}
	return nil, false
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

	f()

	// Remove the iptable rule
	removeCmd := exec.Command(iptables, remove...)
	err = removeCmd.Start()
	assert.Nil(t, err)
	err = removeCmd.Wait()
	assert.Nil(t, err)
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
