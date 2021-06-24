// +build linux_bpf windows

package tracer

import (
	"bufio"
	"bytes"
	"context"
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
	"syscall"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/http"
	"github.com/DataDog/datadog-agent/pkg/network/testutil"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cihub/seelog"
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
	dnsSupported := dnsSupported(t)
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
		if section == "dns" && !dnsSupported {
			// DNS stats not supported on some systems
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
	if !dnsSupported(t) {
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
	sendCount := connectionBufferCapacity(tr)/4 + 1
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

	preCap := connectionBufferCapacity(tr)
	connections := getConnections(t, tr)
	src := connections.Conns[0].SPort
	dst := connections.Conns[0].DPort
	t.Logf("got %d connections", len(connections.Conns))
	// ensure we didn't grow or shrink the buffer
	require.Equal(t, preCap, connectionBufferCapacity(tr))

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
	require.Equal(t, preCap, connectionBufferCapacity(tr))
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
	if !httpSupported(t) {
		t.Skip("HTTP monitoring not supported")
	}

	cfg := testConfig()
	cfg.EnableHTTPMonitoring = true
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()
}

func TestHTTPStats(t *testing.T) {
	if !httpSupported(t) {
		t.Skip("HTTP monitoring feature not available")
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

	// forked child should have exited, and only the parent should remain
	require.Eventually(t, func() bool {
		conns := searchConnections(getConnections(t, tr), func(cs network.ConnectionStats) bool {
			return cs.SPort == uint16(port) && cs.DPort == uint16(port) && cs.Source.IsLoopback() && cs.Dest.IsLoopback()
		})

		t.Logf("connections: %v", conns)
		return len(conns) == 1
	}, 5*time.Second, time.Second, "could not find expected number of tcp connections, expected: 1")
}
