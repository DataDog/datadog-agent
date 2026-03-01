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
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

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
	log.SetupLogger(log.Default(), logLevel)
	platformInit()
	os.Exit(m.Run())
}

type TracerSuite struct {
	suite.Suite
}

func SupportedNetworkBuildModes() []ebpftest.BuildMode {
	modes := ebpftest.SupportedBuildModes()
	if !slices.Contains(modes, ebpftest.Ebpfless) {
		modes = append(modes, ebpftest.Ebpfless)
	}
	return modes
}

func TestTracerSuite(t *testing.T) {
	ebpftest.TestBuildModes(t, SupportedNetworkBuildModes(), "", func(t *testing.T) {
		suite.Run(t, new(TracerSuite))
	})
}

func setupTracer(t testing.TB, cfg *config.Config) *Tracer {
	if ebpftest.GetBuildMode() == ebpftest.Ebpfless {
		env.SetFeatures(t, env.ECSFargate)
		// protocol classification not yet supported on fargate
		cfg.ProtocolClassificationEnabled = false
	}
	if ebpftest.GetBuildMode() == ebpftest.Fentry {
		cfg.ProtocolClassificationEnabled = false
	}

	tr, err := NewTracer(cfg, nil, nil)
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
		testutil.GracefulCloseTCP(c)
	})
	t.Cleanup(server.Shutdown)
	err := server.Run()
	require.NoError(t, err)

	c, err := server.Dial()
	require.NoError(t, err)
	defer testutil.GracefulCloseTCP(c)

	// Connect to server 10 times
	wg := new(errgroup.Group)
	for i := 0; i < 10; i++ {
		wg.Go(func() error {
			// Write clientMessageSize to server, and read response
			if _, err := c.Write(genPayload(clientMessageSize)); err != nil {
				return err
			}

			r := bufio.NewReader(c)
			r.ReadBytes(byte('\n'))
			return nil
		})
	}

	err = wg.Wait()
	require.NoError(t, err)

	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		// Iterate through active connections until we find connection created above, and confirm send + recv counts
		connections, cleanup := getConnections(collect, tr)
		defer cleanup()
		conn, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
		require.True(collect, ok)
		require.NotNil(collect, conn)

		m := conn.Monotonic
		assert.Equal(collect, 10*clientMessageSize, int(m.SentBytes))
		assert.Equal(collect, 10*serverMessageSize, int(m.RecvBytes))
		if !cfg.EnableEbpfless {
			assert.Equal(collect, os.Getpid(), int(conn.Pid))
		}
		assert.Equal(collect, addrPort(server.Address()), int(conn.DPort))
		assert.Equal(collect, network.OUTGOING, conn.Direction)
	}, 4*time.Second, 100*time.Millisecond, "failed to find connection")

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
	c, err := server.Dial()
	require.NoError(t, err)

	// Write clientMessageSize to server, and read response
	_, err = c.Write(genPayload(clientMessageSize))
	require.NoError(t, err)
	r := bufio.NewReader(c)
	r.ReadBytes(byte('\n'))

	// Explicitly close this TCP connection
	c.Close()

	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		connections, cleanup := getConnections(collect, tr)
		defer cleanup()
		conn, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
		require.True(collect, ok)

		m := conn.Monotonic
		assert.Equal(collect, clientMessageSize, int(m.SentBytes))
		assert.Equal(collect, serverMessageSize, int(m.RecvBytes))
		assert.Equal(collect, 0, int(m.Retransmits))
		if !tr.config.EnableEbpfless {
			assert.Equal(collect, os.Getpid(), int(conn.Pid))
		}
		assert.Equal(collect, addrPort(server.Address()), int(conn.DPort))
		assert.Equal(collect, network.OUTGOING, conn.Direction)
		assert.True(collect, conn.IntraHost)

		// Verify the short lived connection is accounting for both TCP_ESTABLISHED and TCP_CLOSED events
		assert.Equal(collect, uint16(1), m.TCPEstablished)
		assert.Equal(collect, uint16(1), m.TCPClosed)
		assert.Empty(collect, conn.TCPFailures, "connection should have no failures")
	}, 3*time.Second, 100*time.Millisecond, "connection not found")

	connections, cleanup := getConnections(t, tr)
	defer cleanup()
	_, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
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
			testutil.GracefulCloseTCP(c)
		}
	}()

	// Connect to server
	c, err := testutil.DialTCP("tcp6", ln.Addr().String())
	require.NoError(t, err)

	// Write clientMessageSize to server, and read response
	if _, err = c.Write(genPayload(clientMessageSize)); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(c)
	r.ReadBytes(byte('\n'))

	connections, cleanup := getConnections(t, tr)
	defer cleanup()

	conn, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	require.True(t, ok)
	m := conn.Monotonic
	assert.Equal(t, clientMessageSize, int(m.SentBytes))
	assert.Equal(t, serverMessageSize, int(m.RecvBytes))
	assert.Equal(t, 0, int(m.Retransmits))
	if !tr.config.EnableEbpfless {
		assert.Equal(t, os.Getpid(), int(conn.Pid))
	}
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
		testutil.GracefulCloseTCP(c)
	})

	t.Cleanup(server.Shutdown)
	require.NoError(t, server.Run())

	// Connect to server
	c, err := server.Dial()
	if err != nil {
		t.Fatal(err)
	}
	defer testutil.GracefulCloseTCP(c)

	// Write clientMessageSize to server, and read response
	if _, err = c.Write(genPayload(clientMessageSize)); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(c)
	r.ReadBytes(byte('\n'))

	connections, cleanup := getConnections(t, tr)
	defer cleanup()

	// Confirm that we could not find connection created above
	_, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	require.False(t, ok)
}

// tests the case of empty TCP connections
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
	c, err := server.Dial()
	require.NoError(t, err)
	<-processedChan
	c.Close()

	var forward *network.ConnectionStats
	var reverse *network.ConnectionStats
	// for ebpfless, it takes time for the packet capture to arrive, so poll
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		// Test
		connections, cleanup := getConnections(collect, tr)
		defer cleanup()

		// Server-side
		newForward, _ := findConnection(c.RemoteAddr(), c.LocalAddr(), connections)
		if newForward != nil {
			forward = newForward
		}
		// Client-side
		newReverse, _ := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
		if newReverse != nil {
			reverse = newReverse
		}

		require.NotNil(collect, forward)
		require.NotNil(collect, reverse)

		require.Equal(collect, network.INCOMING, forward.Direction)
		require.Equal(collect, network.OUTGOING, reverse.Direction)
		require.Equal(collect, uint16(1), forward.Monotonic.TCPEstablished)
		require.Equal(collect, uint16(1), forward.Monotonic.TCPClosed)
		require.Equal(collect, uint16(1), reverse.Monotonic.TCPEstablished)
		require.Equal(collect, uint16(1), reverse.Monotonic.TCPClosed)

		require.Empty(t, forward.TCPFailures, "forward should have no failures")
		require.Empty(t, reverse.TCPFailures, "reverse should have no failures")
	}, 3*time.Second, 100*time.Millisecond, "connection not found")

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
			testUDPSendAndReceive(t, tr, "udp", "127.0.0.1:8081")
		})
	})
	t.Run("v6", func(t *testing.T) {
		if !testConfig().CollectUDPv6Conns {
			t.Skip("UDPv6 disabled")
		}
		t.Run("fixed port", func(t *testing.T) {
			testUDPSendAndReceive(t, tr, "udp6", "[::1]:8081")
		})
	})
}

func testUDPSendAndReceive(t *testing.T, tr *Tracer, ntwk, addr string) {
	tr.removeClient(clientID)

	server := &UDPServer{
		network: ntwk,
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
	c, err := server.Dial()
	require.NoError(t, err)
	defer c.Close()

	// Write clientMessageSize to server, and read response
	_, err = c.Write(genPayload(clientMessageSize))
	require.NoError(t, err)

	_, err = c.Read(make([]byte, serverMessageSize))
	require.NoError(t, err)

	var incoming, outgoing *network.ConnectionStats
	// Iterate through active connections until we find connection created above, and confirm send + recv counts
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		// use t instead of ct because getConnections uses require (not assert), and we get a better error message
		connections, cleanup := getConnections(ct, tr)
		defer cleanup()
		curIncoming, ok := findConnection(c.RemoteAddr(), c.LocalAddr(), connections)
		if ok {
			incoming = curIncoming
		}
		if assert.NotNil(ct, incoming, "unable to find incoming connection") {
			assert.Equal(ct, network.INCOMING, incoming.Direction)

			// make sure the inverse values are seen for the other message
			assert.Equal(ct, serverMessageSize, int(incoming.Monotonic.SentBytes), "incoming sent")
			assert.Equal(ct, clientMessageSize, int(incoming.Monotonic.RecvBytes), "incoming recv")
			assert.True(ct, incoming.IntraHost, "incoming intrahost")
		}

		curOutgoing, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
		if ok {
			outgoing = curOutgoing
		}
		if assert.NotNil(ct, outgoing, "unable to find outgoing connection") {
			assert.Equal(ct, network.OUTGOING, outgoing.Direction)

			assert.Equal(ct, clientMessageSize, int(outgoing.Monotonic.SentBytes), "outgoing sent")
			assert.Equal(ct, serverMessageSize, int(outgoing.Monotonic.RecvBytes), "outgoing recv")
			assert.True(ct, outgoing.IntraHost, "outgoing intrahost")
		}

	}, 4*time.Second, 100*time.Millisecond)
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
		network: "udp",
		onMessage: func([]byte, int) []byte {
			return genPayload(serverMessageSize)
		},
	}

	err := server.Run(clientMessageSize)
	require.NoError(t, err)
	t.Cleanup(server.Shutdown)

	// Connect to server
	c, err := server.Dial()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Write clientMessageSize to server, and read response
	if _, err = c.Write(genPayload(clientMessageSize)); err != nil {
		t.Fatal(err)
	}

	_, err = c.Read(make([]byte, serverMessageSize))
	require.NoError(t, err)

	// Iterate through active connections until we find connection created above, and confirm send + recv counts
	connections, cleanup := getConnections(t, tr)
	defer cleanup()

	_, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	require.False(t, ok)
}

func (s *TracerSuite) TestLocalDNSCollectionDisabled() {
	t := s.T()
	// Enable BPF-based system probe with DNS disabled (by default)
	config := testConfig()

	tr := setupTracer(t, config)

	// Connect to local DNS
	cn, err := dialUDP("udp", "127.0.0.1:53")
	assert.NoError(t, err)
	defer cn.Close()

	// Write anything
	_, err = cn.Write([]byte("test"))
	assert.NoError(t, err)

	// Iterate through active connections making sure there are no local DNS calls
	connections, cleanup := getConnections(t, tr)
	defer cleanup()
	for _, c := range connections.Conns {
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
	cn, err := dialUDP("udp", "127.0.0.1:53")
	assert.NoError(t, err)
	defer cn.Close()

	// Write anything
	_, err = cn.Write([]byte("test"))
	assert.NoError(t, err)

	// Iterate through active connections making sure theres at least one connection
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		connections, cleanup := getConnections(collect, tr)
		defer cleanup()
		for _, c := range connections.Conns {
			if isLocalDNS(c) {
				return
			}
		}

		require.Fail(collect, "could not find connection")
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
	cn, err := dialUDP("udp", "127.0.0.1:80")
	assert.NoError(t, err)
	defer cn.Close()

	// Write anything
	_, err = cn.Write([]byte("test"))
	assert.NoError(t, err)

	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		// Make sure we're not picking up 127.0.0.1:80
		cxs, cleanup := getConnections(collect, tr)
		defer cleanup()
		for _, c := range cxs.Conns {
			assert.False(collect, c.Source.String() == "127.0.0.1" && c.SPort == 80, "connection %s should be excluded", c)
			assert.False(collect, c.Dest.String() == "127.0.0.1" && c.DPort == 80 && c.Type == network.TCP, "connection %s should be excluded", c)
		}

		// ensure one of the connections is UDP to 127.0.0.1:80
		assert.Condition(collect, func() bool {
			for _, c := range cxs.Conns {
				if c.Dest.String() == "127.0.0.1" && c.DPort == 80 && c.Type == network.UDP {
					return true
				}
			}
			return false
		}, "Unable to find UDP connection to 127.0.0.1:80")

	}, 2*time.Second, 100*time.Millisecond)
}

func (s *TracerSuite) TestShouldExcludeEmptyStatsConnection() {
	t := s.T()
	cfg := testConfig()
	tr := setupTracer(t, cfg)

	// Connect to 127.0.0.1:80
	cn, err := dialUDP("udp", "127.0.0.1:80")
	assert.NoError(t, err)
	defer cn.Close()

	// Write anything
	_, err = cn.Write([]byte("test"))
	assert.NoError(t, err)

	var zeroConn network.ConnectionStats
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		cxs, cleanup := getConnections(collect, tr)
		defer cleanup()
		for _, c := range cxs.Conns {
			if c.Dest.String() == "127.0.0.1" && c.DPort == 80 {
				zeroConn = c
				return
			}
		}
		require.Fail(collect, "could not find connection")
	}, 2*time.Second, 100*time.Millisecond)

	// next call should not have the same connection
	cxs, cleanup := getConnections(t, tr)
	defer cleanup()
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
		tr := &Tracer{config: &config.Config{CollectLocalDNS: false, DNSMonitoringPortList: []int{53}}}
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
		tr := &Tracer{config: &config.Config{CollectLocalDNS: true, DNSMonitoringPortList: []int{53}}}

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
		server := &UDPServer{
			network:   "udp",
			onMessage: echoOnMessage,
		}
		err := server.Run(size)
		require.NoError(b, err)
		defer server.Shutdown()

		c, err := server.Dial()
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
				testutil.GracefulCloseTCP(c)
				return
			}
			c.Write(buf)
		}
	}

	return func(b *testing.B) {
		server := testutil.NewTCPServer(echoOnMessage)
		b.Cleanup(server.Shutdown)
		require.NoError(b, server.Run())

		c, err := server.Dial()
		if err != nil {
			b.Fatal(err)
		}
		defer testutil.GracefulCloseTCP(c)
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
				testutil.GracefulCloseTCP(c)
				return
			}
		}
	}

	return func(b *testing.B) {
		server := testutil.NewTCPServer(dropOnMessage)
		b.Cleanup(server.Shutdown)
		require.NoError(b, server.Run())

		c, err := server.Dial()
		if err != nil {
			b.Fatal(err)
		}
		defer testutil.GracefulCloseTCP(c)

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
	if s.network == "" {
		return errors.New("must set network for UDPServer.Run()")
	}
	var err error
	var ln net.PacketConn
	if s.lc != nil {
		ln, err = s.lc.ListenPacket(context.Background(), s.network, s.address)
	} else {
		ln, err = net.ListenPacket(s.network, s.address)
	}
	if err != nil {
		return err
	}
	err = ln.SetDeadline(time.Now().Add(time.Minute))
	if err != nil {
		ln.Close()
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
				_, err = ln.WriteTo(ret, addr)
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

func (s *UDPServer) Dial() (net.Conn, error) {
	return dialUDP(s.network, s.address)
}

func (s *UDPServer) Shutdown() {
	if s.ln != nil {
		_ = s.ln.Close()
		s.ln = nil
	}
}

func dialUDP(network, address string) (net.Conn, error) {
	if network == "" {
		return nil, errors.New("must set network to dialUDP")
	}
	conn, err := net.DialTimeout(network, address, 50*time.Millisecond)
	if err != nil {
		return nil, err
	}
	err = testutil.SetTestDeadline(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
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

func getConnections(t require.TestingT, tr *Tracer) (*network.Connections, func()) {
	// Iterate through active connections until we find connection created above, and confirm send + recv counts
	connections, cleanup, err := tr.GetActiveConnections(clientID)
	require.NoError(t, err)
	return connections, cleanup
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
		require.NoError(c, err)
		dnsClientAddr := dnsConn.LocalAddr().(*net.UDPAddr)
		_, _, err = dnsClient.ExchangeWithConn(queryMsg, dnsConn)
		if timeout == 0 {
			require.NoError(c, err, "unexpected error making DNS request")
		} else {
			require.Error(c, err)
		}
		_ = dnsConn.Close()
		require.NoError(c, tr.reverseDNS.WaitForDomain(domain))

		// Iterate through active connections until we find connection created above, and confirm send + recv counts
		connections, cleanup := getConnections(c, tr)
		defer cleanup()
		conn, ok := findConnection(dnsClientAddr, dnsServerAddr, connections)
		if passed := assert.True(c, ok); !passed {
			return
		}

		require.Equal(c, queryMsg.Len(), int(conn.Monotonic.SentBytes))
		if !tr.config.EnableEbpfless {
			require.Equal(c, os.Getpid(), int(conn.Pid))
		}
		require.Equal(c, dnsServerAddr.Port, int(conn.DPort))

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
		require.Equal(c, uint32(success), successfulResponses, "expected %d successful responses but got %d", success, successfulResponses)
		require.Equal(c, uint32(failure), failedResponses)
		require.Equal(c, uint32(timeout), timeouts, "expected %d timeouts but got %d", timeout, timeouts)
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
	cfg := testConfig()

	tr := setupTracer(t, cfg)

	server := testutil.NewTCPServer(func(c net.Conn) {
		io.Copy(io.Discard, c)
		c.Close()
	})
	t.Cleanup(server.Shutdown)
	require.NoError(t, server.Run())

	c, err := server.Dial()
	require.NoError(t, err)

	laddr, raddr := c.LocalAddr(), c.RemoteAddr()
	c.Write([]byte("hello"))

	var conn *network.ConnectionStats
	var ok bool

	// for ebpfless, wait for the packet capture to appear
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		connections, cleanup := getConnections(collect, tr)
		defer cleanup()
		conn, ok = findConnection(laddr, raddr, connections)
		require.True(collect, ok)
	}, 3*time.Second, 100*time.Millisecond, "couldn't find connection")

	require.True(t, ok)
	assert.Equal(t, uint16(1), conn.Last.TCPEstablished)
	assert.Equal(t, uint16(0), conn.Last.TCPClosed)

	c.Close()

	// Wait for the connection to be sent from the perf buffer
	time.Sleep(100 * time.Millisecond)
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		var ok bool
		connections, cleanup := getConnections(collect, tr)
		defer cleanup()
		conn, ok = findConnection(laddr, raddr, connections)
		require.True(collect, ok)
	}, 3*time.Second, 100*time.Millisecond, "couldn't find connection")

	require.True(t, ok)
	assert.Equal(t, uint16(0), conn.Last.TCPEstablished)
	assert.Equal(t, uint16(1), conn.Last.TCPClosed)
	assert.Empty(t, conn.TCPFailures, "connection should have no failures")
}

func (s *TracerSuite) TestTCPEstablishedPreExistingConn() {
	t := s.T()
	server := testutil.NewTCPServer(func(c net.Conn) {
		io.Copy(io.Discard, c)
		c.Close()
	})
	t.Cleanup(server.Shutdown)
	require.NoError(t, server.Run())

	c, err := server.Dial()
	require.NoError(t, err)
	laddr, raddr := c.LocalAddr(), c.RemoteAddr()
	t.Logf("laddr=%s raddr=%s", laddr, raddr)

	// Ensure closed connections are flushed as soon as possible
	cfg := testConfig()

	tr := setupTracer(t, cfg)

	c.Write([]byte("hello"))
	c.Close()

	// Wait for the connection to be sent from the perf buffer
	time.Sleep(100 * time.Millisecond)
	var conn *network.ConnectionStats
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		var ok bool
		connections, cleanup := getConnections(collect, tr)
		defer cleanup()
		conn, ok = findConnection(laddr, raddr, connections)
		require.True(collect, ok)
	}, 3*time.Second, 100*time.Millisecond, "couldn't find connection")

	m := conn.Monotonic
	assert.Equal(t, uint16(0), m.TCPEstablished)
	assert.Equal(t, uint16(1), m.TCPClosed)
	assert.Empty(t, conn.TCPFailures, "connection should have no failures")
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
	err = testutil.SetTestDeadline(conn)
	require.NoError(t, err)
	message := []byte("payload")
	bytesSent, err := conn.WriteTo(message, remoteAddr)
	require.NoError(t, err)

	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		connections, cleanup := getConnections(ct, tr)
		defer cleanup()
		outgoing := network.FilterConnections(connections, func(cs network.ConnectionStats) bool {
			return cs.DPort == uint16(remotePort)
		})

		require.Len(ct, outgoing, 1)
		assert.Equal(ct, bytesSent, int(outgoing[0].Monotonic.SentBytes))
	}, 3*time.Second, 100*time.Millisecond)
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
	conn, err := dialUDP("udp6", remoteAddr.String())
	require.NoError(t, err)
	defer conn.Close()
	message := []byte("payload")
	bytesSent, err := conn.Write(message)
	require.NoError(t, err)

	var outgoing []network.ConnectionStats
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		connections, cleanup := getConnections(ct, tr)
		defer cleanup()
		outgoing = network.FilterConnections(connections, func(cs network.ConnectionStats) bool {
			return cs.DPort == uint16(remotePort)
		})
		require.Len(ct, outgoing, 1)

		assert.Equal(ct, remoteAddr.IP.String(), outgoing[0].Dest.String())
		assert.Equal(ct, bytesSent, int(outgoing[0].Monotonic.SentBytes))
	}, 3*time.Second, 100*time.Millisecond, "failed to find connection")

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
	require.EventuallyWithTf(t, func(collect *assert.CollectT) {
		conns, cleanup := getConnections(collect, tr)
		defer cleanup()
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

		require.Len(collect, outgoingConns, 1)
		require.Len(collect, incomingConns, 1)
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
	conn, err := testutil.DialTCP("tcp", srvAddr)
	if err == nil {
		conn.Close() // If the connection unexpectedly succeeds, close it immediately.
		require.Fail(t, "expected connection to be refused, but it succeeded")
	}
	require.Error(t, err, "expected connection refused error but got none")

	// Check if the connection was recorded as refused
	var foundConn *network.ConnectionStats
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		conns, cleanup := getConnections(collect, tr)
		defer cleanup()
		// Check for the refusal record
		foundConn = findFailedConnectionByRemoteAddr(srvAddr, conns, 111)
		require.NotNil(collect, foundConn)
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
	c, err := testutil.DialTCP("tcp", serverAddr)
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
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		connections, cleanup := getConnections(collect, tr)
		defer cleanup()
		// 104 is the errno for ECONNRESET
		// findFailedConnection needs `t` for logging, hence no need to pass `collect`.
		conn = findFailedConnection(t, c.LocalAddr().String(), serverAddr, connections, 104)
		require.NotNil(collect, conn)
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
	c, err := testutil.DialTCP("tcp", serverAddr)
	require.NoError(t, err, "could not connect to server: ", err)

	// Wait briefly to give the server time to close the connection
	time.Sleep(50 * time.Millisecond)

	// Attempt to write to the server, expecting a reset
	_, writeErr := c.Write([]byte("ping"))
	require.Error(t, writeErr, "expected connection reset error but got none")

	// Check if the connection was recorded as reset
	var conn *network.ConnectionStats
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		conns, cleanup := getConnections(collect, tr)
		defer cleanup()
		// 104 is the errno for ECONNRESET
		// findFailedConnection needs `t` for logging, hence no need to pass `collect`.
		conn = findFailedConnection(t, c.LocalAddr().String(), serverAddr, conns, 104)
		require.NotNil(collect, conn)
	}, 3*time.Second, 100*time.Millisecond, "Failed connection not recorded properly")

	require.NoError(t, c.Close(), "error closing client connection")

	assert.Equal(t, uint32(1), conn.TCPFailures[104], "expected 1 connection reset")
	assert.Equal(t, uint32(0), conn.TCPFailures[111], "expected 0 connection refused")
	assert.Equal(t, uint32(0), conn.TCPFailures[110], "expected 0 connection timeout")
	assert.Equal(t, uint64(0), conn.Monotonic.SentBytes, "expected 0 bytes sent")
	assert.Equal(t, uint64(0), conn.Monotonic.RecvBytes, "expected 0 bytes received")
}

// findFailedConnection is a utility function to find a failed connection based on specific TCP error codes
func findFailedConnection(t *testing.T, local, remote string, conns *network.Connections, errorCode uint16) *network.ConnectionStats { // nolint:unused
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
func findFailedConnectionByRemoteAddr(remoteAddr string, conns *network.Connections, errorCode uint16) *network.ConnectionStats {
	failureFilter := func(cs network.ConnectionStats) bool {
		return netip.MustParseAddrPort(remoteAddr) == netip.AddrPortFrom(cs.Dest.Addr, cs.DPort) && cs.TCPFailures[errorCode] > 0
	}
	return network.FirstConnection(conns, failureFilter)
}

func BenchmarkGetActiveConnections(b *testing.B) {
	cfg := testConfig()
	tr := setupTracer(b, cfg)
	server := testutil.NewTCPServer(func(c net.Conn) {
		testutil.GracefulCloseTCP(c)
	})
	b.Cleanup(server.Shutdown)
	require.NoError(b, server.Run())

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		c, err := server.Dial()
		require.NoError(b, err)
		laddr, raddr := c.LocalAddr(), c.RemoteAddr()
		c.Write([]byte("hello"))
		connections, _ := getConnections(b, tr)
		conn, ok := findConnection(laddr, raddr, connections)

		require.True(b, ok)
		assert.Equal(b, uint32(1), conn.Last.TCPEstablished)
		assert.Equal(b, uint32(0), conn.Last.TCPClosed)
		testutil.GracefulCloseTCP(c)

		// Wait for the connection to be sent from the perf buffer
		require.Eventually(b, func() bool {
			var ok bool
			connections, _ := getConnections(b, tr)
			conn, ok = findConnection(laddr, raddr, connections)
			return ok
		}, 3*time.Second, 10*time.Millisecond, "couldn't find connection")

		require.True(b, ok)
		assert.Equal(b, uint32(0), conn.Last.TCPEstablished)
		assert.Equal(b, uint32(1), conn.Last.TCPClosed)
	}
}
