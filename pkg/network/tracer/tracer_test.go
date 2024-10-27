// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf || (windows && npm)

package tracer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	nethttp "net/http"
	"net/netip"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cihub/seelog"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/sync/errgroup"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/testdns"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	clientMessageSize = 2 << 8
	serverMessageSize = 2 << 14
	payloadSizesTCP   = []int{2 << 5, 2 << 8, 2 << 10, 2 << 12, 2 << 14, 2 << 15}
	payloadSizesUDP   = []int{2 << 5, 2 << 8, 2 << 12, 2 << 14}
)

func TestMain(m *testing.M) {
	logLevel := os.Getenv("DD_LOG_LEVEL")
	if logLevel == "" {
		logLevel = "warn"
	}
	log.SetupLogger(seelog.Default, logLevel)
	platformInit()
	os.Exit(m.Run())
}

type TracerSuite struct {
	suite.Suite
}

func TestTracerSuite(t *testing.T) {
	ebpftest.TestBuildModes(t, ebpftest.SupportedBuildModes(), "", func(t *testing.T) {
		suite.Run(t, new(TracerSuite))
	})
}

func isFentry() bool {
	return ebpftest.GetBuildMode() == ebpftest.Fentry
}

func setupTracer(t testing.TB, cfg *config.Config) *Tracer {
	if isFentry() {
		env.SetFeatures(t, env.ECSFargate)
		// protocol classification not yet supported on fargate
		cfg.ProtocolClassificationEnabled = false
	}

	tr, err := NewTracer(cfg, nil)
	require.NoError(t, err)
	t.Cleanup(tr.Stop)

	initTracerState(t, tr)
	return tr
}

func (s *TracerSuite) TestGetStats() {
	t := s.T()
	httpSupported := httpSupported()
	linuxExpected := map[string]interface{}{}
	err := json.Unmarshal([]byte(`{
      "state": {
        "closed_conn_dropped": 0,
		"conn_dropped": 0
      },
      "tracer": {
        "runtime": {
          "runtime_compilation_enabled": 0
        }
      }
    }`), &linuxExpected)
	require.NoError(t, err)

	rcExceptions := map[string]interface{}{}
	err = json.Unmarshal([]byte(`{
      "conntrack": {
        "evicts_total": 0,
        "orphan_size": 0
      }}`), &rcExceptions)
	require.NoError(t, err)

	expected := linuxExpected
	if runtime.GOOS == "windows" {
		expected = map[string]interface{}{
			"state": map[string]interface{}{},
		}
	}

	for _, enableEbpfConntracker := range []bool{true, false} {
		t.Run(fmt.Sprintf("ebpf conntracker %v", enableEbpfConntracker), func(t *testing.T) {
			cfg := testConfig()
			cfg.EnableHTTPMonitoring = true
			cfg.EnableEbpfConntracker = enableEbpfConntracker
			tr := setupTracer(t, cfg)

			<-time.After(time.Second)

			getConnections(t, tr)
			actual, _ := tr.getStats()

			for section, entries := range expected {
				if section == "usm" && !httpSupported {
					// HTTP stats not supported on some systems
					continue
				}
				require.Contains(t, actual, section, "missing section from telemetry map: %s", section)
				for name := range entries.(map[string]interface{}) {
					if cfg.EnableRuntimeCompiler || cfg.EnableEbpfConntracker {
						if sec, ok := rcExceptions[section]; ok {
							if _, ok := sec.(map[string]interface{})[name]; ok {
								continue
							}
						}
					}
					assert.Contains(t, actual[section], name, "%s actual is missing %s", section, name)
				}
			}
		})
	}
}

func (s *TracerSuite) TestTCPSendAndReceive() {
	t := s.T()
	cfg := testConfig()
	tr := setupTracer(t, cfg)

	// Create TCP Server which, for every line, sends back a message with size=serverMessageSize
	server := testutil.NewTCPServer(func(c net.Conn) {
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
	t.Cleanup(server.Shutdown)
	err := server.Run()
	require.NoError(t, err)

	c, err := net.DialTimeout("tcp", server.Address(), 50*time.Millisecond)
	require.NoError(t, err)
	defer c.Close()

	// Connect to server 10 times
	wg := new(errgroup.Group)
	for i := 0; i < 10; i++ {
		wg.Go(func() error {
			// Write clientMessageSize to server, and read response
			if _, err = c.Write(genPayload(clientMessageSize)); err != nil {
				return err
			}

			r := bufio.NewReader(c)
			r.ReadBytes(byte('\n'))
			return nil
		})
	}

	err = wg.Wait()
	require.NoError(t, err)

	var conn *network.ConnectionStats
	require.Eventually(t, func() bool {
		// Iterate through active connections until we find connection created above, and confirm send + recv counts
		connections := getConnections(t, tr)
		var ok bool
		conn, ok = findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
		return conn != nil && ok
	}, 3*time.Second, 100*time.Millisecond, "failed to find connection")

	m := conn.Monotonic
	assert.Equal(t, 10*clientMessageSize, int(m.SentBytes))
	assert.Equal(t, 10*serverMessageSize, int(m.RecvBytes))
	if !cfg.EnableEbpfless {
		assert.Equal(t, os.Getpid(), int(conn.Pid))
	}
	assert.Equal(t, addrPort(server.Address()), int(conn.DPort))
	assert.Equal(t, network.OUTGOING, conn.Direction)
}

func (s *TracerSuite) TestTCPShortLived() {
	t := s.T()
	// Enable BPF-based system probe
	cfg := testConfig()
	tr := setupTracer(t, cfg)

	// Create TCP Server which sends back serverMessageSize bytes
	server := testutil.NewTCPServer(func(c net.Conn) {
		r := bufio.NewReader(c)
		r.ReadBytes(byte('\n'))
		c.Write(genPayload(serverMessageSize))
		c.Close()
	})
	t.Cleanup(server.Shutdown)
	require.NoError(t, server.Run())

	// Connect to server
	c, err := net.DialTimeout("tcp", server.Address(), 50*time.Millisecond)
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
	}, 3*time.Second, 100*time.Millisecond, "connection not found")

	m := conn.Monotonic
	assert.Equal(t, clientMessageSize, int(m.SentBytes))
	assert.Equal(t, serverMessageSize, int(m.RecvBytes))
	assert.Equal(t, 0, int(m.Retransmits))
	assert.Equal(t, os.Getpid(), int(conn.Pid))
	assert.Equal(t, addrPort(server.Address()), int(conn.DPort))
	assert.Equal(t, network.OUTGOING, conn.Direction)
	assert.True(t, conn.IntraHost)

	// Verify the short lived connection is accounting for both TCP_ESTABLISHED and TCP_CLOSED events
	assert.Equal(t, uint16(1), m.TCPEstablished)
	assert.Equal(t, uint16(1), m.TCPClosed)

	_, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), getConnections(t, tr))
	assert.False(t, ok)
}

func (s *TracerSuite) TestTCPOverIPv6() {
	t := s.T()
	t.SkipNow()
	cfg := testConfig()
	cfg.CollectTCPv6Conns = true
	tr := setupTracer(t, cfg)

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
	m := conn.Monotonic
	assert.Equal(t, clientMessageSize, int(m.SentBytes))
	assert.Equal(t, serverMessageSize, int(m.RecvBytes))
	assert.Equal(t, 0, int(m.Retransmits))
	assert.Equal(t, os.Getpid(), int(conn.Pid))
	assert.Equal(t, ln.Addr().(*net.TCPAddr).Port, int(conn.DPort))
	assert.Equal(t, network.OUTGOING, conn.Direction)
	assert.True(t, conn.IntraHost)

	doneChan <- struct{}{}
}

func (s *TracerSuite) TestTCPCollectionDisabled() {
	t := s.T()
	if runtime.GOOS == "windows" {
		t.Skip("Test disabled on Windows")
	}
	// Enable BPF-based system probe with TCP disabled
	cfg := testConfig()
	cfg.CollectTCPv4Conns = false
	cfg.CollectTCPv6Conns = false
	tr := setupTracer(t, cfg)

	// Create TCP Server which sends back serverMessageSize bytes
	server := testutil.NewTCPServer(func(c net.Conn) {
		r := bufio.NewReader(c)
		r.ReadBytes(byte('\n'))
		c.Write(genPayload(serverMessageSize))
		c.Close()
	})

	t.Cleanup(server.Shutdown)
	require.NoError(t, server.Run())

	// Connect to server
	c, err := net.DialTimeout("tcp", server.Address(), 50*time.Millisecond)
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

func (s *TracerSuite) TestTCPConnsReported() {
	t := s.T()
	// Setup
	cfg := testConfig()
	cfg.CollectTCPv4Conns = true
	cfg.CollectTCPv6Conns = true
	tr := setupTracer(t, cfg)

	processedChan := make(chan struct{})
	server := testutil.NewTCPServer(func(c net.Conn) {
		c.Close()
		close(processedChan)
	})
	t.Cleanup(server.Shutdown)
	require.NoError(t, server.Run())

	// Connect to server
	c, err := net.DialTimeout("tcp", server.Address(), 50*time.Millisecond)
	require.NoError(t, err)
	defer c.Close()
	<-processedChan

	// Test
	connections := getConnections(t, tr)
	// Server-side
	_, ok := findConnection(c.RemoteAddr(), c.LocalAddr(), connections)
	require.True(t, ok)
	// Client-side
	_, ok = findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	require.True(t, ok)
}

func (s *TracerSuite) TestUDPSendAndReceive() {
	t := s.T()
	cfg := testConfig()
	tr := setupTracer(t, cfg)

	t.Run("v4", func(t *testing.T) {
		if !testConfig().CollectUDPv4Conns {
			t.Skip("UDPv4 disabled")
		}
		t.Run("fixed port", func(t *testing.T) {
			testUDPSendAndReceive(t, tr, "127.0.0.1:8081")
		})
	})
	t.Run("v6", func(t *testing.T) {
		if !testConfig().CollectUDPv6Conns {
			t.Skip("UDPv6 disabled")
		}
		t.Run("fixed port", func(t *testing.T) {
			testUDPSendAndReceive(t, tr, "[::1]:8081")
		})
	})
}

func testUDPSendAndReceive(t *testing.T, tr *Tracer, addr string) {
	tr.removeClient(clientID)

	server := &UDPServer{
		address: addr,
		onMessage: func(_ []byte, _ int) []byte {
			return genPayload(serverMessageSize)
		},
	}

	err := server.Run(clientMessageSize)
	require.NoError(t, err)
	t.Cleanup(server.Shutdown)

	initTracerState(t, tr)

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

func (s *TracerSuite) TestUDPDisabled() {
	t := s.T()
	// Enable BPF-based system probe with UDP disabled
	cfg := testConfig()
	cfg.CollectUDPv4Conns = false
	cfg.CollectUDPv6Conns = false
	tr := setupTracer(t, cfg)

	// Create UDP Server which sends back serverMessageSize bytes
	server := &UDPServer{
		onMessage: func([]byte, int) []byte {
			return genPayload(serverMessageSize)
		},
	}

	err := server.Run(clientMessageSize)
	require.NoError(t, err)
	t.Cleanup(server.Shutdown)

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

func (s *TracerSuite) TestLocalDNSCollectionDisabled() {
	t := s.T()
	// Enable BPF-based system probe with DNS disabled (by default)
	config := testConfig()

	tr := setupTracer(t, config)

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

func (s *TracerSuite) TestLocalDNSCollectionEnabled() {
	t := s.T()
	// Enable BPF-based system probe with DNS enabled
	cfg := testConfig()
	cfg.CollectLocalDNS = true
	cfg.CollectUDPv4Conns = true
	tr := setupTracer(t, cfg)

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
	require.Eventually(t, func() bool {
		for _, c := range getConnections(t, tr).Conns {
			if isLocalDNS(c) {
				return true
			}
		}

		return false
	}, 3*time.Second, 100*time.Millisecond, "could not find connection")
}

func isLocalDNS(c network.ConnectionStats) bool {
	return c.Source.String() == "127.0.0.1" && c.Dest.String() == "127.0.0.1" && c.DPort == 53
}

func (s *TracerSuite) TestShouldSkipExcludedConnection() {
	t := s.T()
	// exclude connections from 127.0.0.1:80
	cfg := testConfig()
	// exclude source SSH connections to make this pass in VM
	cfg.ExcludedSourceConnections = map[string][]string{"127.0.0.1": {"80"}, "*": {"22"}}
	cfg.ExcludedDestinationConnections = map[string][]string{"127.0.0.1": {"tcp 80"}}
	tr := setupTracer(t, cfg)

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
	cxs := getConnections(t, tr)
	for _, c := range cxs.Conns {
		assert.False(t, c.Source.String() == "127.0.0.1" && c.SPort == 80, "connection %s should be excluded", c)
		assert.False(t, c.Dest.String() == "127.0.0.1" && c.DPort == 80 && c.Type == network.TCP, "connection %s should be excluded", c)
	}

	// ensure one of the connections is UDP to 127.0.0.1:80
	assert.Condition(t, func() bool {
		for _, c := range cxs.Conns {
			if c.Dest.String() == "127.0.0.1" && c.DPort == 80 && c.Type == network.UDP {
				return true
			}
		}
		return false
	}, "Unable to find UDP connection to 127.0.0.1:80")
}

func (s *TracerSuite) TestShouldExcludeEmptyStatsConnection() {
	t := s.T()
	cfg := testConfig()
	tr := setupTracer(t, cfg)

	// Connect to 127.0.0.1:80
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:80")
	assert.NoError(t, err)

	cn, err := net.DialUDP("udp", nil, addr)
	assert.NoError(t, err)
	defer cn.Close()

	// Write anything
	_, err = cn.Write([]byte("test"))
	assert.NoError(t, err)

	var zeroConn network.ConnectionStats
	require.Eventually(t, func() bool {
		cxs := getConnections(t, tr)
		for _, c := range cxs.Conns {
			if c.Dest.String() == "127.0.0.1" && c.DPort == 80 {
				zeroConn = c
				return true
			}
		}
		return false
	}, 2*time.Second, 100*time.Millisecond)

	// next call should not have the same connection
	cxs := getConnections(t, tr)
	found := false
	for _, c := range cxs.Conns {
		if c.Source == zeroConn.Source && c.SPort == zeroConn.SPort &&
			c.Dest == zeroConn.Dest && c.DPort == zeroConn.DPort &&
			c.Pid == zeroConn.Pid {
			found = true
			break
		}
	}
	require.False(t, found, "empty connections should be filtered out")
}

func TestSkipConnectionDNS(t *testing.T) {
	t.Run("CollectLocalDNS disabled", func(t *testing.T) {
		tr := &Tracer{config: &config.Config{CollectLocalDNS: false}}
		assert.True(t, tr.shouldSkipConnection(&network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
			Source: util.AddressFromString("10.0.0.1"),
			Dest:   util.AddressFromString("127.0.0.1"),
			SPort:  1000, DPort: 53,
		}}))

		assert.False(t, tr.shouldSkipConnection(&network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
			Source: util.AddressFromString("10.0.0.1"),
			Dest:   util.AddressFromString("127.0.0.1"),
			SPort:  1000, DPort: 8080,
		}}))

		assert.True(t, tr.shouldSkipConnection(&network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
			Source: util.AddressFromString("::3f::45"),
			Dest:   util.AddressFromString("::1"),
			SPort:  53, DPort: 1000,
		}}))

		assert.True(t, tr.shouldSkipConnection(&network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
			Source: util.AddressFromString("::3f::45"),
			Dest:   util.AddressFromString("::1"),
			SPort:  53, DPort: 1000,
		}}))
	})

	t.Run("CollectLocalDNS disabled", func(t *testing.T) {
		tr := &Tracer{config: &config.Config{CollectLocalDNS: true}}

		assert.False(t, tr.shouldSkipConnection(&network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
			Source: util.AddressFromString("10.0.0.1"),
			Dest:   util.AddressFromString("127.0.0.1"),
			SPort:  1000, DPort: 53,
		}}))

		assert.False(t, tr.shouldSkipConnection(&network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
			Source: util.AddressFromString("10.0.0.1"),
			Dest:   util.AddressFromString("127.0.0.1"),
			SPort:  1000, DPort: 8080,
		}}))

		assert.False(t, tr.shouldSkipConnection(&network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
			Source: util.AddressFromString("::3f::45"),
			Dest:   util.AddressFromString("::1"),
			SPort:  53, DPort: 1000,
		}}))

		assert.False(t, tr.shouldSkipConnection(&network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
			Source: util.AddressFromString("::3f::45"),
			Dest:   util.AddressFromString("::1"),
			SPort:  53, DPort: 1000,
		}}))
	})
}

func findConnection(l, r net.Addr, c *network.Connections) (*network.ConnectionStats, bool) {
	res := network.FirstConnection(c, network.ByTuple(l, r))
	return res, res != nil
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
	_ = setupTracer(b, testConfig())

	runBenchtests(b, payloadSizesUDP, "eBPF", benchEchoUDP)
}

func benchEchoUDP(size int) func(b *testing.B) {
	payload := genPayload(size)
	echoOnMessage := func(b []byte, _ int) []byte {
		resp := make([]byte, len(b))
		copy(resp, b)
		return resp
	}

	return func(b *testing.B) {
		server := &UDPServer{onMessage: echoOnMessage}
		err := server.Run(size)
		require.NoError(b, err)
		defer server.Shutdown()

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
	_ = setupTracer(b, testConfig())
	runBenchtests(b, payloadSizesTCP, "eBPF", benchEchoTCP)
}

func BenchmarkTCPSend(b *testing.B) {
	runBenchtests(b, payloadSizesTCP, "", benchSendTCP)

	// Enable BPF-based system probe
	_ = setupTracer(b, testConfig())
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
		server := testutil.NewTCPServer(echoOnMessage)
		b.Cleanup(server.Shutdown)
		require.NoError(b, server.Run())

		c, err := net.DialTimeout("tcp", server.Address(), 50*time.Millisecond)
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
		server := testutil.NewTCPServer(dropOnMessage)
		b.Cleanup(server.Shutdown)
		require.NoError(b, server.Run())

		c, err := net.DialTimeout("tcp", server.Address(), 50*time.Millisecond)
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

type UDPServer struct {
	network   string
	address   string
	lc        *net.ListenConfig
	onMessage func(b []byte, n int) []byte
	ln        net.PacketConn
}

func (s *UDPServer) Run(payloadSize int) error {
	networkType := "udp"
	if s.network != "" {
		networkType = s.network
	}
	var err error
	var ln net.PacketConn
	if s.lc != nil {
		ln, err = s.lc.ListenPacket(context.Background(), networkType, s.address)
	} else {
		ln, err = net.ListenPacket(networkType, s.address)
	}
	if err != nil {
		return err
	}

	s.ln = ln
	s.address = s.ln.LocalAddr().String()

	go func() {
		buf := make([]byte, payloadSize)
		for {
			n, addr, err := ln.ReadFrom(buf)
			if err != nil {
				if !errors.Is(err, net.ErrClosed) {
					fmt.Printf("readfrom: %s\n", err)
				}
				return
			}
			ret := s.onMessage(buf, n)
			if ret != nil {
				_, err = s.ln.WriteTo(ret, addr)
				if err != nil {
					if !errors.Is(err, net.ErrClosed) {
						fmt.Printf("writeto: %s\n", err)
					}
					return
				}
			}
		}
	}()

	return nil
}

func (s *UDPServer) Shutdown() {
	if s.ln != nil {
		_ = s.ln.Close()
		s.ln = nil
	}
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

func addrPort(addr string) int {
	p, _ := strconv.Atoi(strings.Split(addr, ":")[1])
	return p
}

const clientID = "1"

func initTracerState(t testing.TB, tr *Tracer) {
	err := tr.RegisterClient(clientID)
	require.NoError(t, err)
}

func getConnections(t require.TestingT, tr *Tracer) *network.Connections {
	// Iterate through active connections until we find connection created above, and confirm send + recv counts
	connections, err := tr.GetActiveConnections(clientID)
	require.NoError(t, err)
	return connections
}

func testDNSStats(t *testing.T, tr *Tracer, domain string, success, failure, timeout int, serverIP string) {
	tr.removeClient(clientID)
	initTracerState(t, tr)

	dnsServerAddr := &net.UDPAddr{IP: net.ParseIP(serverIP), Port: 53}

	queryMsg := new(dns.Msg)
	queryMsg.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	queryMsg.RecursionDesired = true

	// we place the entire test in the eventually loop since failed DNS requests can cause "WaitForDomain" to return
	// true before DNS data is actually available for the successful connection
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		dnsClient := new(dns.Client)
		dnsConn, err := dnsClient.Dial(dnsServerAddr.String())
		if !assert.NoError(c, err) {
			return
		}
		dnsClientAddr := dnsConn.LocalAddr().(*net.UDPAddr)
		_, _, err = dnsClient.ExchangeWithConn(queryMsg, dnsConn)
		if timeout == 0 {
			if !assert.NoError(c, err, "unexpected error making DNS request") {
				return
			}
		} else {
			if !assert.Error(c, err) {
				return
			}
		}
		_ = dnsConn.Close()
		if !assert.NoError(c, tr.reverseDNS.WaitForDomain(domain)) {
			return
		}

		// Iterate through active connections until we find connection created above, and confirm send + recv counts
		connections := getConnections(c, tr)
		conn, ok := findConnection(dnsClientAddr, dnsServerAddr, connections)
		if passed := assert.True(c, ok); !passed {
			return
		}

		if !assert.Equal(c, queryMsg.Len(), int(conn.Monotonic.SentBytes)) {
			return
		}
		if !tr.config.EnableEbpfless {
			if !assert.Equal(c, os.Getpid(), int(conn.Pid)) {
				return
			}
		}
		if !assert.Equal(c, dnsServerAddr.Port, int(conn.DPort)) {
			return
		}

		var total uint32
		var successfulResponses uint32
		var timeouts uint32
		for _, byDomain := range conn.DNSStats {
			for _, byQueryType := range byDomain {
				successfulResponses += byQueryType.CountByRcode[uint32(0)]
				timeouts += byQueryType.Timeouts
				for _, count := range byQueryType.CountByRcode {
					total += count
				}
			}
		}
		failedResponses := total - successfulResponses

		// DNS Stats
		if !assert.Equal(c, uint32(success), successfulResponses, "expected %d successful responses but got %d", success, successfulResponses) {
			return
		}
		if !assert.Equal(c, uint32(failure), failedResponses) {
			return
		}
		if !assert.Equal(c, uint32(timeout), timeouts, "expected %d timeouts but got %d", timeout, timeouts) {
			return
		}
	}, 10*time.Second, 100*time.Millisecond, "Failed to get dns response or unexpected response")
}

func (s *TracerSuite) TestDNSStats() {
	t := s.T()
	cfg := testConfig()
	cfg.CollectDNSStats = true
	cfg.DNSTimeout = 1 * time.Second
	cfg.CollectLocalDNS = true
	tr := setupTracer(t, cfg)
	t.Run("valid domain", func(t *testing.T) {
		testDNSStats(t, tr, "good.com", 1, 0, 0, testdns.GetServerIP(t).String())
	})
	t.Run("invalid domain", func(t *testing.T) {
		testDNSStats(t, tr, "abcdedfg", 0, 1, 0, testdns.GetServerIP(t).String())
	})
	t.Run("timeout", func(t *testing.T) {
		testDNSStats(t, tr, "golang.org", 0, 0, 1, "1.2.3.4")
	})
}

func (s *TracerSuite) TestTCPEstablished() {
	t := s.T()
	// Ensure closed connections are flushed as soon as possible
	cfg := testConfig()

	tr := setupTracer(t, cfg)

	server := testutil.NewTCPServer(func(c net.Conn) {
		io.Copy(io.Discard, c)
		c.Close()
	})
	t.Cleanup(server.Shutdown)
	require.NoError(t, server.Run())

	c, err := net.DialTimeout("tcp", server.Address(), 50*time.Millisecond)
	require.NoError(t, err)

	laddr, raddr := c.LocalAddr(), c.RemoteAddr()
	c.Write([]byte("hello"))

	connections := getConnections(t, tr)
	conn, ok := findConnection(laddr, raddr, connections)

	require.True(t, ok)
	assert.Equal(t, uint16(1), conn.Last.TCPEstablished)
	assert.Equal(t, uint16(0), conn.Last.TCPClosed)

	c.Close()

	// Wait for the connection to be sent from the perf buffer
	require.Eventually(t, func() bool {
		var ok bool
		conn, ok = findConnection(laddr, raddr, getConnections(t, tr))
		return ok
	}, 3*time.Second, 100*time.Millisecond, "couldn't find connection")

	require.True(t, ok)
	assert.Equal(t, uint16(0), conn.Last.TCPEstablished)
	assert.Equal(t, uint16(1), conn.Last.TCPClosed)
}

func (s *TracerSuite) TestTCPEstablishedPreExistingConn() {
	t := s.T()
	server := testutil.NewTCPServer(func(c net.Conn) {
		io.Copy(io.Discard, c)
		c.Close()
	})
	t.Cleanup(server.Shutdown)
	require.NoError(t, server.Run())

	c, err := net.DialTimeout("tcp", server.Address(), 50*time.Millisecond)
	require.NoError(t, err)
	laddr, raddr := c.LocalAddr(), c.RemoteAddr()

	// Ensure closed connections are flushed as soon as possible
	cfg := testConfig()

	tr := setupTracer(t, cfg)

	c.Write([]byte("hello"))
	c.Close()

	// Wait for the connection to be sent from the perf buffer
	var conn *network.ConnectionStats
	require.Eventually(t, func() bool {
		var ok bool
		conn, ok = findConnection(laddr, raddr, getConnections(t, tr))
		return ok
	}, 3*time.Second, 100*time.Millisecond, "couldn't find connection")

	m := conn.Monotonic
	assert.Equal(t, uint16(0), m.TCPEstablished)
	assert.Equal(t, uint16(1), m.TCPClosed)
}

func (s *TracerSuite) TestUnconnectedUDPSendIPv4() {
	t := s.T()
	cfg := testConfig()
	tr := setupTracer(t, cfg)

	remotePort := rand.Int()%5000 + 15000
	remoteAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: remotePort}
	// Use ListenUDP instead of DialUDP to create a "connectionless" UDP connection
	conn, err := net.ListenUDP("udp4", nil)
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
	assert.Equal(t, bytesSent, int(outgoing[0].Monotonic.SentBytes))
}

func (s *TracerSuite) TestConnectedUDPSendIPv6() {
	t := s.T()
	cfg := testConfig()
	if !testConfig().CollectUDPv6Conns {
		t.Skip("UDPv6 disabled")
	}
	tr := setupTracer(t, cfg)

	remotePort := rand.Int()%5000 + 15000
	remoteAddr := &net.UDPAddr{IP: net.IPv6loopback, Port: remotePort}
	conn, err := net.DialUDP("udp6", nil, remoteAddr)
	require.NoError(t, err)
	defer conn.Close()
	message := []byte("payload")
	bytesSent, err := conn.Write(message)
	require.NoError(t, err)

	var outgoing []network.ConnectionStats
	require.Eventually(t, func() bool {
		connections := getConnections(t, tr)
		outgoing = network.FilterConnections(connections, func(cs network.ConnectionStats) bool {
			return cs.DPort == uint16(remotePort)
		})

		return len(outgoing) == 1
	}, 3*time.Second, 100*time.Millisecond, "failed to find connection")

	require.Len(t, outgoing, 1)
	assert.Equal(t, remoteAddr.IP.String(), outgoing[0].Dest.String())
	assert.Equal(t, bytesSent, int(outgoing[0].Monotonic.SentBytes))
}

func (s *TracerSuite) TestTCPDirection() {
	t := s.T()
	cfg := testConfig()
	tr := setupTracer(t, cfg)

	// Start an HTTP server on localhost:8080
	serverAddr := "127.0.0.1:8080"
	srv := &nethttp.Server{
		Addr: serverAddr,
		Handler: nethttp.HandlerFunc(func(w nethttp.ResponseWriter, req *nethttp.Request) {
			t.Logf("received http request from %s", req.RemoteAddr)
			io.Copy(io.Discard, req.Body)
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
			outgoingConns = network.FilterConnections(conns, func(cs network.ConnectionStats) bool {
				return fmt.Sprintf("%s:%d", cs.Dest, cs.DPort) == serverAddr
			})
		}
		if len(incomingConns) == 0 {
			incomingConns = network.FilterConnections(conns, func(cs network.ConnectionStats) bool {
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

func (s *TracerSuite) TestTCPFailureConnectionRefused() {
	t := s.T()

	checkSkipFailureConnectionsTests(t)

	cfg := testConfig()
	cfg.TCPFailedConnectionsEnabled = true
	tr := setupTracer(t, cfg)

	// try to connect to a port where no server is accepting connections
	srvAddr := "127.0.0.1:9998"
	conn, err := net.Dial("tcp", srvAddr)
	if err == nil {
		conn.Close() // If the connection unexpectedly succeeds, close it immediately.
		require.Fail(t, "expected connection to be refused, but it succeeded")
	}
	require.Error(t, err, "expected connection refused error but got none")

	// Check if the connection was recorded as refused
	var foundConn *network.ConnectionStats
	require.Eventually(t, func() bool {
		conns := getConnections(t, tr)
		// Check for the refusal record
		foundConn = findFailedConnectionByRemoteAddr(srvAddr, conns, 111)
		return foundConn != nil
	}, 3*time.Second, 100*time.Millisecond, "Failed connection not recorded properly")

	assert.Equal(t, uint32(1), foundConn.TCPFailures[111], "expected 1 connection refused")
	assert.Equal(t, uint32(0), foundConn.TCPFailures[104], "expected 0 connection reset")
	assert.Equal(t, uint32(0), foundConn.TCPFailures[110], "expected 0 connection timeout")
	assert.Equal(t, uint64(0), foundConn.Monotonic.SentBytes, "expected 0 bytes sent")
	assert.Equal(t, uint64(0), foundConn.Monotonic.RecvBytes, "expected 0 bytes received")
}

func (s *TracerSuite) TestTCPFailureConnectionResetWithData() {
	t := s.T()

	checkSkipFailureConnectionsTests(t)

	cfg := testConfig()
	cfg.TCPFailedConnectionsEnabled = true
	tr := setupTracer(t, cfg)

	srv := testutil.NewTCPServer(func(c net.Conn) {
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

	serverAddr := srv.Address()
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

func (s *TracerSuite) TestTCPFailureConnectionResetNoData() {
	t := s.T()

	checkSkipFailureConnectionsTests(t)

	cfg := testConfig()
	cfg.TCPFailedConnectionsEnabled = true
	tr := setupTracer(t, cfg)

	// Server that immediately resets the connection without any data transfer
	srv := testutil.NewTCPServer(func(c net.Conn) {
		if tcpConn, ok := c.(*net.TCPConn); ok {
			tcpConn.SetLinger(0)
		}
		time.Sleep(10 * time.Millisecond)
		// Close the connection immediately to trigger a reset
		c.Close()
	})

	require.NoError(t, srv.Run(), "error running server")
	t.Cleanup(srv.Shutdown)

	serverAddr := srv.Address()
	c, err := net.Dial("tcp", serverAddr)
	require.NoError(t, err, "could not connect to server: ", err)

	// Wait briefly to give the server time to close the connection
	time.Sleep(50 * time.Millisecond)

	// Attempt to write to the server, expecting a reset
	_, writeErr := c.Write([]byte("ping"))
	require.Error(t, writeErr, "expected connection reset error but got none")

	// Check if the connection was recorded as reset
	var conn *network.ConnectionStats
	require.Eventually(t, func() bool {
		conns := getConnections(t, tr)
		// 104 is the errno for ECONNRESET
		conn = findFailedConnection(t, c.LocalAddr().String(), serverAddr, conns, 104)
		return conn != nil
	}, 3*time.Second, 100*time.Millisecond, "Failed connection not recorded properly")

	require.NoError(t, c.Close(), "error closing client connection")

	assert.Equal(t, uint32(1), conn.TCPFailures[104], "expected 1 connection reset")
	assert.Equal(t, uint32(0), conn.TCPFailures[111], "expected 0 connection refused")
	assert.Equal(t, uint32(0), conn.TCPFailures[110], "expected 0 connection timeout")
	assert.Equal(t, uint64(0), conn.Monotonic.SentBytes, "expected 0 bytes sent")
	assert.Equal(t, uint64(0), conn.Monotonic.RecvBytes, "expected 0 bytes received")
}

// findFailedConnection is a utility function to find a failed connection based on specific TCP error codes
func findFailedConnection(t *testing.T, local, remote string, conns *network.Connections, errorCode uint32) *network.ConnectionStats { // nolint:unused
	// Extract the address and port from the net.Addr types
	localAddrPort, err := netip.ParseAddrPort(local)
	if err != nil {
		t.Logf("Failed to parse local address: %v", err)
		return nil
	}
	remoteAddrPort, err := netip.ParseAddrPort(remote)
	if err != nil {
		t.Logf("Failed to parse remote address: %v", err)
		return nil
	}

	failureFilter := func(cs network.ConnectionStats) bool {
		localMatch := netip.AddrPortFrom(cs.Source.Addr, cs.SPort) == localAddrPort
		remoteMatch := netip.AddrPortFrom(cs.Dest.Addr, cs.DPort) == remoteAddrPort
		return localMatch && remoteMatch && cs.TCPFailures[errorCode] > 0
	}

	return network.FirstConnection(conns, failureFilter)
}

// for some failed connections we don't know the local addr/port so we need to search by remote addr only
func findFailedConnectionByRemoteAddr(remoteAddr string, conns *network.Connections, errorCode uint32) *network.ConnectionStats {
	failureFilter := func(cs network.ConnectionStats) bool {
		return netip.MustParseAddrPort(remoteAddr) == netip.AddrPortFrom(cs.Dest.Addr, cs.DPort) && cs.TCPFailures[errorCode] > 0
	}
	return network.FirstConnection(conns, failureFilter)
}

func BenchmarkGetActiveConnections(b *testing.B) {
	cfg := testConfig()
	tr := setupTracer(b, cfg)
	server := testutil.NewTCPServer(func(c net.Conn) {
		io.Copy(io.Discard, c)
		c.Close()
	})
	b.Cleanup(server.Shutdown)
	require.NoError(b, server.Run())

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		c, err := net.DialTimeout("tcp", server.Address(), 50*time.Millisecond)
		require.NoError(b, err)
		laddr, raddr := c.LocalAddr(), c.RemoteAddr()
		c.Write([]byte("hello"))
		connections := getConnections(b, tr)
		conn, ok := findConnection(laddr, raddr, connections)

		require.True(b, ok)
		assert.Equal(b, uint32(1), conn.Last.TCPEstablished)
		assert.Equal(b, uint32(0), conn.Last.TCPClosed)
		c.Close()

		// Wait for the connection to be sent from the perf buffer
		require.Eventually(b, func() bool {
			var ok bool
			conn, ok = findConnection(laddr, raddr, getConnections(b, tr))
			return ok
		}, 3*time.Second, 10*time.Millisecond, "couldn't find connection")

		require.True(b, ok)
		assert.Equal(b, uint32(0), conn.Last.TCPEstablished)
		assert.Equal(b, uint32(1), conn.Last.TCPClosed)
	}
}
