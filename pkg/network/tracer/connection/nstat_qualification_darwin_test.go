// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package connection

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/nstat"
)

const (
	nstatQualificationEnv  = "RUN_NSTAT_FUNCTIONAL_TEST"
	nstatHelperModeEnv     = "DD_NSTAT_TEST_HELPER_MODE"
	nstatHelperAddressEnv  = "DD_NSTAT_TEST_HELPER_ADDRESS"
	nstatHelperRequestEnv  = "DD_NSTAT_TEST_HELPER_REQUEST"
	nstatHelperResponseEnv = "DD_NSTAT_TEST_HELPER_RESPONSE"
	nstatHelperHoldEnv     = "DD_NSTAT_TEST_HELPER_HOLD"
	nstatHelperWaitEnv     = "DD_NSTAT_TEST_HELPER_WAIT"
	nstatUDPReady          = "R"
	nstatUDPRelease        = "X"
)

type nstatClosedCollector struct {
	mu    sync.Mutex
	conns []network.ConnectionStats
}

func (c *nstatClosedCollector) add(conn *network.ConnectionStats) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conns = append(c.conns, *conn)
}

func (c *nstatClosedCollector) count(pid uint32, localPort, remotePort uint16) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	count := 0
	for _, conn := range c.conns {
		if conn.Pid == pid && conn.SPort == localPort && conn.DPort == remotePort {
			count++
		}
	}
	return count
}

func (c *nstatClosedCollector) find(pid uint32, remotePort uint16) (network.ConnectionStats, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, conn := range c.conns {
		if conn.Pid == pid && conn.DPort == remotePort {
			return conn, true
		}
	}
	return network.ConnectionStats{}, false
}

func (c *nstatClosedCollector) findTuple(pid uint32, localPort, remotePort uint16) (network.ConnectionStats, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, conn := range c.conns {
		if conn.Pid == pid && conn.SPort == localPort && conn.DPort == remotePort {
			return conn, true
		}
	}
	return network.ConnectionStats{}, false
}

func TestNStatQualificationSeparateProcessPayloadAndLifecycle(t *testing.T) {
	requireNStatQualification(t)

	for _, tc := range []struct {
		name    string
		network string
		address string
	}{
		{name: "tcp_ipv4", network: "tcp4", address: "127.0.0.1:0"},
		{name: "tcp_ipv6", network: "tcp6", address: "[::1]:0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tracer, closed := startNStatQualificationTracer(t)
			listener, err := net.Listen(tc.network, tc.address)
			if err != nil && tc.network == "tcp6" {
				t.Skipf("IPv6 loopback unavailable: %v", err)
			}
			require.NoError(t, err)
			defer listener.Close()

			request := "nstat-separate-process-request-" + tc.name
			response := "nstat-separate-process-response-" + tc.name
			cmd, output := startNStatHelper(t, tc.network, listener.Addr().String(), request, response)

			server, err := listener.Accept()
			require.NoError(t, err)
			clientAddress := server.RemoteAddr().(*net.TCPAddr)
			serverAddress := server.LocalAddr().(*net.TCPAddr)
			require.Equal(t, request, readExactString(t, server, len(request)))

			pid := uint32(cmd.Process.Pid)
			conn := requireNStatConnection(
				t,
				tracer,
				pid,
				uint16(clientAddress.Port),
				uint16(serverAddress.Port),
				network.TCP,
				uint64(len(request)),
				0,
			)
			require.Equal(t, uint64(len(request)), conn.Monotonic.SentBytes)
			require.Equal(t, network.OUTGOING, conn.Direction)

			serverConn := requireNStatConnection(
				t,
				tracer,
				uint32(os.Getpid()),
				uint16(serverAddress.Port),
				uint16(clientAddress.Port),
				network.TCP,
				0,
				uint64(len(request)),
			)
			require.Equal(t, network.INCOMING, serverConn.Direction)

			_, err = io.WriteString(server, response)
			require.NoError(t, err)
			conn = requireNStatConnection(
				t,
				tracer,
				pid,
				uint16(clientAddress.Port),
				uint16(serverAddress.Port),
				network.TCP,
				uint64(len(request)),
				uint64(len(response)),
			)
			require.Equal(t, uint64(len(request)), conn.Monotonic.SentBytes)
			require.Equal(t, uint64(len(response)), conn.Monotonic.RecvBytes)
			require.NoError(t, server.Close())
			require.NoErrorf(t, cmd.Wait(), "helper failed: %s", output.String())

			require.Eventually(t, func() bool {
				return closed.count(pid, uint16(clientAddress.Port), uint16(serverAddress.Port)) == 1
			}, 5*time.Second, 10*time.Millisecond)
			require.Never(t, func() bool {
				return closed.count(pid, uint16(clientAddress.Port), uint16(serverAddress.Port)) > 1
			}, 250*time.Millisecond, 10*time.Millisecond)
		})
	}

	for _, tc := range []struct {
		name    string
		network string
		address string
	}{
		{name: "udp_ipv4", network: "udp4", address: "127.0.0.1:0"},
		{name: "udp_ipv6", network: "udp6", address: "[::1]:0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tracer, _ := startNStatQualificationTracer(t)
			server, err := net.ListenPacket(tc.network, tc.address)
			if err != nil && tc.network == "udp6" {
				t.Skipf("IPv6 loopback unavailable: %v", err)
			}
			require.NoError(t, err)
			defer server.Close()

			request := "nstat-datagram-request-" + tc.name
			response := "nstat-datagram-response-" + tc.name
			cmd, output := startNStatHelper(t, tc.network, server.LocalAddr().String(), request, response)

			buffer := make([]byte, 1024)
			require.NoError(t, server.SetReadDeadline(time.Now().Add(5*time.Second)))
			n, clientAddress, err := server.ReadFrom(buffer)
			require.NoError(t, err)
			require.Equal(t, request, string(buffer[:n]))

			clientPort := uint16(clientAddress.(*net.UDPAddr).Port)
			serverPort := uint16(server.LocalAddr().(*net.UDPAddr).Port)
			pid := uint32(cmd.Process.Pid)

			_, err = server.WriteTo([]byte(response), clientAddress)
			require.NoError(t, err)
			require.NoError(t, server.SetReadDeadline(time.Now().Add(5*time.Second)))
			n, readyAddress, err := server.ReadFrom(buffer)
			require.NoError(t, err)
			require.Equal(t, nstatUDPReady, string(buffer[:n]))
			require.Equal(t, clientAddress.String(), readyAddress.String())
			require.NoError(t, tracer.control.QueryAll())

			expectedSentBytes := uint64(len(request) + len(nstatUDPReady))
			conn := requireNStatConnection(
				t,
				tracer,
				pid,
				clientPort,
				serverPort,
				network.UDP,
				expectedSentBytes,
				uint64(len(response)),
			)
			require.Equal(t, expectedSentBytes, conn.Monotonic.SentBytes)
			require.Equal(t, uint64(len(response)), conn.Monotonic.RecvBytes)

			_, err = server.WriteTo([]byte(nstatUDPRelease), clientAddress)
			require.NoError(t, err)
			require.NoErrorf(t, cmd.Wait(), "helper failed: %s", output.String())
		})
	}
}

func TestNStatQualificationOneWayUDP(t *testing.T) {
	requireNStatQualification(t)

	for _, tc := range []struct {
		name    string
		network string
		address string
	}{
		{name: "ipv4", network: "udp4", address: "127.0.0.1:0"},
		{name: "ipv6", network: "udp6", address: "[::1]:0"},
	} {
		t.Run(tc.name+"_open", func(t *testing.T) {
			tracer, closed := startNStatQualificationTracer(t)
			server, err := net.ListenPacket(tc.network, tc.address)
			if err != nil && tc.network == "udp6" {
				t.Skipf("IPv6 loopback unavailable: %v", err)
			}
			require.NoError(t, err)
			defer server.Close()

			request := "nstat-one-way-open-" + tc.name
			cmd, output, release := startHeldNStatHelper(t, tc.network, server.LocalAddr().String(), request)
			defer release.Close()

			buffer := make([]byte, 1024)
			require.NoError(t, server.SetReadDeadline(time.Now().Add(5*time.Second)))
			n, clientAddress, err := server.ReadFrom(buffer)
			require.NoError(t, err)
			require.Equal(t, request, string(buffer[:n]))

			clientPort := uint16(clientAddress.(*net.UDPAddr).Port)
			serverPort := uint16(server.LocalAddr().(*net.UDPAddr).Port)
			pid := uint32(cmd.Process.Pid)
			require.NoError(t, tracer.control.QueryAll())
			conn := requireNStatConnection(
				t,
				tracer,
				pid,
				clientPort,
				serverPort,
				network.UDP,
				uint64(len(request)),
				0,
			)
			require.Equal(t, uint64(len(request)), conn.Monotonic.SentBytes)
			require.Zero(t, conn.Monotonic.RecvBytes)

			_, err = io.WriteString(release, nstatUDPRelease)
			require.NoError(t, err)
			require.NoError(t, release.Close())
			require.NoErrorf(t, cmd.Wait(), "helper failed: %s", output.String())
			require.Eventually(t, func() bool {
				return closed.count(pid, clientPort, serverPort) == 1
			}, 5*time.Second, 10*time.Millisecond)
		})

		t.Run(tc.name+"_waiting_response", func(t *testing.T) {
			tracer, closed := startNStatQualificationTracer(t)
			server, err := net.ListenPacket(tc.network, tc.address)
			if err != nil && tc.network == "udp6" {
				t.Skipf("IPv6 loopback unavailable: %v", err)
			}
			require.NoError(t, err)
			defer server.Close()

			request := "nstat-one-way-wait-" + tc.name
			cmd, output := startWaitingNStatHelper(t, tc.network, server.LocalAddr().String(), request)

			buffer := make([]byte, 1024)
			require.NoError(t, server.SetReadDeadline(time.Now().Add(5*time.Second)))
			n, clientAddress, err := server.ReadFrom(buffer)
			require.NoError(t, err)
			require.Equal(t, request, string(buffer[:n]))

			clientPort := uint16(clientAddress.(*net.UDPAddr).Port)
			serverPort := uint16(server.LocalAddr().(*net.UDPAddr).Port)
			pid := uint32(cmd.Process.Pid)
			require.NoError(t, tracer.control.QueryAll())
			conn := requireNStatConnection(
				t,
				tracer,
				pid,
				clientPort,
				serverPort,
				network.UDP,
				uint64(len(request)),
				0,
			)
			require.Equal(t, uint64(len(request)), conn.Monotonic.SentBytes)
			require.Zero(t, conn.Monotonic.RecvBytes)

			_, err = server.WriteTo([]byte(nstatUDPRelease), clientAddress)
			require.NoError(t, err)
			require.NoErrorf(t, cmd.Wait(), "helper failed: %s", output.String())
			require.Eventually(t, func() bool {
				return closed.count(pid, clientPort, serverPort) == 1
			}, 5*time.Second, 10*time.Millisecond)
		})

		t.Run(tc.name+"_immediate_close", func(t *testing.T) {
			tracer, closed := startNStatQualificationTracer(t)
			server, err := net.ListenPacket(tc.network, tc.address)
			if err != nil && tc.network == "udp6" {
				t.Skipf("IPv6 loopback unavailable: %v", err)
			}
			require.NoError(t, err)
			defer server.Close()

			request := "nstat-one-way-close-" + tc.name
			cmd, output := startNStatHelper(t, tc.network, server.LocalAddr().String(), request, "")

			buffer := make([]byte, 1024)
			require.NoError(t, server.SetReadDeadline(time.Now().Add(5*time.Second)))
			n, clientAddress, err := server.ReadFrom(buffer)
			require.NoError(t, err)
			require.Equal(t, request, string(buffer[:n]))
			clientPort := uint16(clientAddress.(*net.UDPAddr).Port)
			serverPort := uint16(server.LocalAddr().(*net.UDPAddr).Port)
			pid := uint32(cmd.Process.Pid)
			require.NoErrorf(t, cmd.Wait(), "helper failed: %s", output.String())
			require.NoError(t, tracer.control.QueryAll())

			var conn network.ConnectionStats
			require.Eventually(t, func() bool {
				var found bool
				conn, found = closed.findTuple(pid, clientPort, serverPort)
				return found
			}, 10*time.Second, 10*time.Millisecond)
			require.True(t, conn.IsClosed)
			require.Equal(t, uint64(len(request)), conn.Monotonic.SentBytes)
			require.Zero(t, conn.Monotonic.RecvBytes)
			require.Never(t, func() bool {
				return closed.count(pid, clientPort, serverPort) > 1
			}, 250*time.Millisecond, 10*time.Millisecond)
		})
	}
}

func TestNStatQualificationShortLivedFlows(t *testing.T) {
	requireNStatQualification(t)

	control, err := nstat.OpenControl()
	require.NoError(t, err)
	tracer := newNStatTracerWithControl(qualificationNStatConfig(), control)
	closed := &nstatClosedCollector{}
	require.NoError(t, tracer.Start(closed.add))
	t.Cleanup(tracer.Stop)
	waitForNStatBaseline(t, tracer)

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	const flowCount = 25
	for i := 0; i < flowCount; i++ {
		request := fmt.Sprintf("short-flow-%d", i)
		cmd, output := startNStatHelper(t, "tcp4", listener.Addr().String(), request, string([]byte{1}))
		server, err := listener.Accept()
		require.NoError(t, err)
		require.Equal(t, request, readExactString(t, server, len(request)))
		clientPort := uint16(server.RemoteAddr().(*net.TCPAddr).Port)
		serverPort := uint16(server.LocalAddr().(*net.TCPAddr).Port)
		_ = requireNStatConnection(
			t,
			tracer,
			uint32(cmd.Process.Pid),
			clientPort,
			serverPort,
			network.TCP,
			uint64(len(request)),
			0,
		)
		_, err = server.Write([]byte{1})
		require.NoError(t, err)
		require.NoError(t, server.Close())
		require.NoErrorf(t, cmd.Wait(), "helper failed: %s", output.String())
	}

	require.Eventually(t, func() bool {
		closed.mu.Lock()
		defer closed.mu.Unlock()
		seen := make(map[network.StatCookie]struct{})
		for _, conn := range closed.conns {
			if conn.DPort == uint16(listener.Addr().(*net.TCPAddr).Port) {
				seen[conn.Cookie] = struct{}{}
			}
		}
		return len(seen) == flowCount
	}, 10*time.Second, 20*time.Millisecond)
}

func TestNStatQualificationRefusedConnection(t *testing.T) {
	requireNStatQualification(t)
	address := os.Getenv("NSTAT_REFUSAL_TARGET")
	if address == "" {
		t.Skip("set NSTAT_REFUSAL_TARGET to a closed TCP port reached through a captured interface")
	}
	remote, err := net.ResolveTCPAddr("tcp", address)
	require.NoError(t, err)
	remotePort := uint16(remote.Port)

	cfg := qualificationNStatConfig()
	cfg.DarwinConnectionTracerPacketEnabled = true
	cfg.DarwinConnectionTracerPacketSnaplen = 8192
	cfg.DarwinConnectionTracerPacketBufferSize = 16 * 1024 * 1024
	cfg.DarwinConnectionTracerLibprocEnabled = false
	tracer, err := newDarwinCompositeTracer(cfg)
	require.NoError(t, err)
	closed := &nstatClosedCollector{}
	require.NoError(t, tracer.Start(closed.add))
	t.Cleanup(tracer.Stop)
	waitForNStatBaseline(t, tracer.primary)

	cmd, output := startNStatHelper(t, "tcp-refused", address, "", "")
	pid := uint32(cmd.Process.Pid)
	require.NoErrorf(t, cmd.Wait(), "helper failed: %s", output.String())

	require.Eventually(t, func() bool {
		conn, ok := closed.find(pid, remotePort)
		return ok && conn.IsClosed && conn.TCPFailures[network.TCPFailureErrnoConnRefused] > 0
	}, 5*time.Second, 10*time.Millisecond)
}

func TestNStatQualificationAbortiveCloseLifecycle(t *testing.T) {
	requireNStatQualification(t)

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	control, err := nstat.OpenControl()
	require.NoError(t, err)
	tracer := newNStatTracerWithControl(qualificationNStatConfig(), control)
	closed := &nstatClosedCollector{}
	require.NoError(t, tracer.Start(closed.add))
	t.Cleanup(tracer.Stop)
	waitForNStatBaseline(t, tracer)

	request := "nstat-reset"
	cmd, output := startNStatHelper(t, "tcp-reset", listener.Addr().String(), request, "")
	server, err := listener.Accept()
	require.NoError(t, err)
	clientPort := uint16(server.RemoteAddr().(*net.TCPAddr).Port)
	serverPort := uint16(server.LocalAddr().(*net.TCPAddr).Port)
	require.Equal(t, request, readExactString(t, server, len(request)))
	pid := uint32(cmd.Process.Pid)
	_ = requireNStatConnection(
		t,
		tracer,
		pid,
		clientPort,
		serverPort,
		network.TCP,
		uint64(len(request)),
		0,
	)
	_, err = server.Write([]byte{1})
	require.NoError(t, err)
	require.NoErrorf(t, cmd.Wait(), "helper failed: %s", output.String())
	_ = server.Close()

	require.Eventually(t, func() bool {
		return closed.count(pid, clientPort, serverPort) == 1
	}, 5*time.Second, 10*time.Millisecond)
}

func TestNStatQualificationSustainedCardinality(t *testing.T) {
	requireNStatQualification(t)
	countText := os.Getenv("NSTAT_LOAD_CONNECTIONS")
	if countText == "" {
		t.Skip("set NSTAT_LOAD_CONNECTIONS to run the sustained cardinality gate")
	}
	count, err := strconv.Atoi(countText)
	require.NoError(t, err)
	require.Positive(t, count)

	control, err := nstat.OpenControl()
	require.NoError(t, err)
	cfg := qualificationNStatConfig()
	cfg.MaxTrackedConnections = max(cfg.MaxTrackedConnections, uint32(count*4))
	tracer := newNStatTracerWithControl(cfg, control)
	closed := &nstatClosedCollector{}
	require.NoError(t, tracer.Start(closed.add))
	t.Cleanup(tracer.Stop)
	waitForNStatBaseline(t, tracer)

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	serverPort := uint16(listener.Addr().(*net.TCPAddr).Port)

	var before runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	started := time.Now()
	clients := make([]net.Conn, 0, count)
	servers := make([]net.Conn, 0, count)
	t.Cleanup(func() {
		for _, conn := range clients {
			_ = conn.Close()
		}
		for _, conn := range servers {
			_ = conn.Close()
		}
	})
	for i := 0; i < count; i++ {
		client, err := net.Dial("tcp4", listener.Addr().String())
		require.NoError(t, err)
		clients = append(clients, client)
		server, err := listener.Accept()
		require.NoError(t, err)
		servers = append(servers, server)
		_, err = client.Write([]byte{byte(i)})
		require.NoError(t, err)
		require.Equal(t, string([]byte{byte(i)}), readExactString(t, server, 1))
	}

	const settleTimeout = 60 * time.Second
	var observed int
	if !assert.Eventually(t, func() bool {
		var buffer network.ConnectionBuffer
		require.NoError(t, tracer.GetConnections(&buffer, nil))
		observed = 0
		for _, conn := range buffer.Connections() {
			if conn.Pid == uint32(os.Getpid()) && conn.DPort == serverPort {
				observed++
			}
		}
		return observed == count
	}, settleTimeout, 20*time.Millisecond) {
		t.Fatalf("observed %d of %d active connections", observed, count)
	}
	creationDuration := time.Since(started)

	var after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&after)
	heapDelta := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	maxHeapMB := int64(128)
	if configured := os.Getenv("NSTAT_MAX_HEAP_DELTA_MB"); configured != "" {
		maxHeapMB, err = strconv.ParseInt(configured, 10, 64)
		require.NoError(t, err)
	}
	require.LessOrEqual(t, heapDelta, maxHeapMB*1024*1024)

	closeStarted := time.Now()
	for i := range clients {
		require.NoError(t, clients[i].Close())
		require.NoError(t, servers[i].Close())
	}
	var closedCount int
	if !assert.Eventually(t, func() bool {
		closed.mu.Lock()
		defer closed.mu.Unlock()
		closedCount = 0
		for _, conn := range closed.conns {
			if conn.Pid == uint32(os.Getpid()) && conn.DPort == serverPort {
				closedCount++
			}
		}
		return closedCount == count
	}, settleTimeout, 20*time.Millisecond) {
		t.Fatalf("observed %d of %d closed connections", closedCount, count)
	}
	t.Logf(
		"connections=%d creation=%s close_latency=%s heap_delta_bytes=%d unresolved=0",
		count,
		creationDuration,
		time.Since(closeStarted),
		heapDelta,
	)
}

func TestNStatQualificationWorkloadHelper(t *testing.T) {
	mode := os.Getenv(nstatHelperModeEnv)
	if mode == "" {
		t.Skip("qualification subprocess helper")
	}

	address := os.Getenv(nstatHelperAddressEnv)
	request := os.Getenv(nstatHelperRequestEnv)
	response := os.Getenv(nstatHelperResponseEnv)
	switch mode {
	case "tcp4", "tcp6":
		conn, err := net.DialTimeout(mode, address, 5*time.Second)
		require.NoError(t, err)
		_, err = io.WriteString(conn, request)
		require.NoError(t, err)
		if response != "" {
			require.Equal(t, response, readExactString(t, conn, len(response)))
			time.Sleep(250 * time.Millisecond)
		}
		require.NoError(t, conn.Close())
	case "tcp-refused":
		conn, err := net.DialTimeout("tcp4", address, 5*time.Second)
		if conn != nil {
			_ = conn.Close()
		}
		require.Error(t, err)
	case "tcp-reset":
		conn, err := net.DialTCP("tcp4", nil, mustResolveTCPAddress(t, address))
		require.NoError(t, err)
		_, err = io.WriteString(conn, request)
		require.NoError(t, err)
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
		require.Equal(t, string([]byte{1}), readExactString(t, conn, 1))
		require.NoError(t, conn.SetLinger(0))
		require.NoError(t, conn.Close())
	case "udp4", "udp6":
		conn, err := net.DialTimeout(mode, address, 5*time.Second)
		require.NoError(t, err)
		_, err = io.WriteString(conn, request)
		require.NoError(t, err)
		if response != "" {
			require.Equal(t, response, readExactString(t, conn, len(response)))
			_, err = io.WriteString(conn, nstatUDPReady)
			require.NoError(t, err)
			require.Equal(t, nstatUDPRelease, readExactString(t, conn, len(nstatUDPRelease)))
		} else if os.Getenv(nstatHelperWaitEnv) == "1" {
			require.Equal(t, nstatUDPRelease, readExactString(t, conn, len(nstatUDPRelease)))
		} else if os.Getenv(nstatHelperHoldEnv) == "1" {
			require.Equal(t, nstatUDPRelease, readExactString(t, os.Stdin, len(nstatUDPRelease)))
		}
		require.NoError(t, conn.Close())
	default:
		t.Fatalf("unknown qualification helper mode %q", mode)
	}
}

func mustResolveTCPAddress(t *testing.T, address string) *net.TCPAddr {
	t.Helper()
	resolved, err := net.ResolveTCPAddr("tcp4", address)
	require.NoError(t, err)
	return resolved
}

func requireNStatQualification(t *testing.T) {
	t.Helper()
	if os.Getenv(nstatQualificationEnv) != "1" {
		t.Skip("set RUN_NSTAT_FUNCTIONAL_TEST=1 to exercise the private kernel control")
	}
}

func qualificationNStatConfig() *config.Config {
	cfg := testNStatConfig()
	cfg.MaxTrackedConnections = 16384
	return cfg
}

func startNStatQualificationTracer(t *testing.T) (*nstatTracer, *nstatClosedCollector) {
	t.Helper()
	control, err := nstat.OpenControl()
	require.NoError(t, err)
	tracer := newNStatTracerWithControl(qualificationNStatConfig(), control)
	closed := &nstatClosedCollector{}
	require.NoError(t, tracer.Start(closed.add))
	t.Cleanup(tracer.Stop)
	waitForNStatBaseline(t, tracer)
	return tracer, closed
}

func waitForNStatBaseline(t *testing.T, tracer *nstatTracer) {
	t.Helper()
	require.NoError(t, tracer.control.QueryAll())
	var sourceCount int
	var undescribed []uint64
	baselineReady := assert.Eventually(t, func() bool {
		tracer.mu.Lock()
		sourceCount = len(tracer.sources)
		undescribed = undescribed[:0]
		for sourceRef, source := range tracer.sources {
			if source.flow == nil {
				undescribed = append(undescribed, sourceRef)
			}
		}
		tracer.mu.Unlock()
		return sourceCount > 0 && len(undescribed) == 0
	}, 3*time.Second, 20*time.Millisecond)
	require.Truef(
		t,
		baselineReady,
		"NStat baseline descriptions did not arrive; sources=%d undescribed_source_refs=%v",
		sourceCount,
		undescribed,
	)
	t.Logf("NStat baseline descriptions received for %d sources", sourceCount)
}

func startNStatHelper(t *testing.T, mode, address, request, response string) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	cmd, output := newNStatHelperCommand(mode, address, request, response)
	startNStatHelperProcess(t, cmd)
	return cmd, output
}

func startHeldNStatHelper(t *testing.T, mode, address, request string) (*exec.Cmd, *bytes.Buffer, io.WriteCloser) {
	t.Helper()
	cmd, output := newNStatHelperCommand(mode, address, request, "")
	cmd.Env = append(cmd.Env, nstatHelperHoldEnv+"=1")
	release, err := cmd.StdinPipe()
	require.NoError(t, err)
	startNStatHelperProcess(t, cmd)
	return cmd, output, release
}

func startWaitingNStatHelper(t *testing.T, mode, address, request string) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	cmd, output := newNStatHelperCommand(mode, address, request, "")
	cmd.Env = append(cmd.Env, nstatHelperWaitEnv+"=1")
	startNStatHelperProcess(t, cmd)
	return cmd, output
}

func newNStatHelperCommand(mode, address, request, response string) (*exec.Cmd, *bytes.Buffer) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestNStatQualificationWorkloadHelper$")
	cmd.Env = append(
		os.Environ(),
		nstatHelperModeEnv+"="+mode,
		nstatHelperAddressEnv+"="+address,
		nstatHelperRequestEnv+"="+request,
		nstatHelperResponseEnv+"="+response,
	)
	output := &bytes.Buffer{}
	cmd.Stdout = output
	cmd.Stderr = output
	return cmd, output
}

func startNStatHelperProcess(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
}

func readExactString(t *testing.T, reader io.Reader, size int) string {
	t.Helper()
	buffer := make([]byte, size)
	_, err := io.ReadFull(reader, buffer)
	require.NoError(t, err)
	return string(buffer)
}

func requireNStatConnection(
	t *testing.T,
	tracer *nstatTracer,
	pid uint32,
	localPort uint16,
	remotePort uint16,
	connType network.ConnectionType,
	sentBytes uint64,
	recvBytes uint64,
) network.ConnectionStats {
	t.Helper()
	var matched network.ConnectionStats
	var candidates []network.ConnectionTuple
	var sources []string
	var lastErr error
	matchedEventually := assert.Eventually(t, func() bool {
		candidates = nil
		sources = nil
		lastErr = nil
		var buffer network.ConnectionBuffer
		if err := tracer.GetConnections(&buffer, nil); err != nil {
			lastErr = err
			return false
		}
		tracer.mu.Lock()
		for sourceRef, source := range tracer.sources {
			if !nstat.IsUDPProvider(source.provider) && (source.flow == nil || source.flow.PID != pid) {
				continue
			}
			sources = append(sources, fmt.Sprintf(
				"ref=%d provider=%d removed=%t flow=%+v",
				sourceRef,
				source.provider,
				source.removed,
				source.flow,
			))
			if len(sources) >= 50 {
				break
			}
		}
		tracer.mu.Unlock()
		for _, conn := range buffer.Connections() {
			if (conn.Pid == pid || conn.SPort == localPort || conn.DPort == remotePort || conn.Type == network.UDP) &&
				len(candidates) < 50 {
				candidates = append(candidates, conn.ConnectionTuple)
			}
			if conn.Pid != pid ||
				conn.SPort != localPort ||
				conn.DPort != remotePort ||
				conn.Type != connType {
				continue
			}
			if conn.Monotonic.SentBytes < sentBytes || conn.Monotonic.RecvBytes < recvBytes {
				continue
			}
			matched = conn
			return true
		}
		return false
	}, 10*time.Second, 10*time.Millisecond)
	require.Truef(
		t,
		matchedEventually,
		"PID %s tuple %d -> %d did not appear; error=%v candidates=%v sources=%v",
		strconv.FormatUint(uint64(pid), 10),
		localPort,
		remotePort,
		lastErr,
		candidates,
		sources,
	)
	return matched
}
