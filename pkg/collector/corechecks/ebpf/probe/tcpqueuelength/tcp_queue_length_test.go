// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package tcpqueuelength

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/tcpqueuelength/model"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var kv = kernel.MustHostVersion()

func TestTCPQueueLengthCompile(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.RuntimeCompiled, "", func(t *testing.T) {
		if kv < kernel.VersionCode(4, 8, 0) {
			t.Skipf("Kernel version %v is not supported by the TCP Queue Length probe", kv)
		}

		cfg := ebpf.NewConfig()
		cfg.BPFDebug = true
		out, err := runtime.TcpQueueLength.Compile(cfg, []string{"-g"}, statsd.Client)
		require.NoError(t, err)
		_ = out.Close()
	})
}

func TestTCPQueueLengthTracer(t *testing.T) {
	ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.RuntimeCompiled, ebpftest.CORE}, "", func(t *testing.T) {
		if kv < kernel.VersionCode(4, 8, 0) {
			t.Skipf("Kernel version %v is not supported by the OOM probe", kv)
		}

		cfg := ebpf.NewConfig()
		tcpTracer, err := NewTracer(cfg)
		require.NoError(t, err)
		t.Cleanup(tcpTracer.Close)

		beforeStats := extractGlobalStats(t, tcpTracer)
		if beforeStats.ReadBufferMaxUsage > 10 {
			t.Errorf("max usage of read buffer is too big before the stress test: %d > 10", beforeStats.ReadBufferMaxUsage)
		}

		err = runTCPLoadTest()
		require.NoError(t, err)
		if total != msgLen {
			require.Equal(t, msgLen, total, "message length")
		}

		afterStats := extractGlobalStats(t, tcpTracer)
		if afterStats.ReadBufferMaxUsage < 1000 {
			t.Errorf("max usage of read buffer is too low after the stress test: %d < 1000", afterStats.ReadBufferMaxUsage)
		}
	})
}

func extractGlobalStats(t *testing.T, tracer *Tracer) model.TCPQueueLengthStatsValue {
	t.Helper()

	stats := tracer.GetAndFlush()
	if stats == nil {
		t.Error("failed to get and flush stats")
	}

	globalStats := model.TCPQueueLengthStatsValue{}

	for cgroup, cgroupStats := range stats {
		t.Logf("%s: read=%d write=%d", cgroup, cgroupStats.ReadBufferMaxUsage, cgroupStats.WriteBufferMaxUsage)
		if cgroupStats.ReadBufferMaxUsage > globalStats.ReadBufferMaxUsage {
			globalStats.ReadBufferMaxUsage = cgroupStats.ReadBufferMaxUsage
		}

		if cgroupStats.WriteBufferMaxUsage > globalStats.WriteBufferMaxUsage {
			globalStats.WriteBufferMaxUsage = cgroupStats.WriteBufferMaxUsage
		}
	}

	return globalStats
}

// TCP test infrastructure
// The idea here is to setup a server and a client, and to slow the server as much as possible by:
// - reading slowly (wait between reads)
// - reading small chunks at a time
// - reducing the RECV buffer size

var Addr *net.TCPAddr = &net.TCPAddr{
	Port: 25568,
}

const msgLen = 10000

var (
	isInSlowMode = true
	total        int
	serverReady  chan struct{}
)

func handleRequest(conn *net.TCPConn) error {
	defer conn.Close()
outer:
	for {
		buf := make([]byte, 10)
		count, err := conn.Read(buf)
		if err != nil {
			return err
		}

		total += count

		for i := 0; i < count; i++ {
			if buf[i] == 0 {
				break outer
			}
		}

		if isInSlowMode {
			time.Sleep(1 * time.Second)
		}
	}

	return nil
}

func server() error {
	listener, err := net.ListenTCP("tcp", Addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	close(serverReady)

	conn, err := listener.AcceptTCP()
	if err != nil {
		return err
	}
	conn.SetReadBuffer(2)

	return handleRequest(conn)
}

func client() error {
	<-serverReady

	conn, err := net.DialTCP("tcp", nil, Addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	msg := make([]byte, msgLen)
	for i := 0; i < msgLen-1; i++ {
		msg[i] = 4
	}
	msg[msgLen-1] = 0

	conn.Write(msg)

	isInSlowMode = false
	return nil
}

func runTCPLoadTest() error {
	serverReady = make(chan struct{})
	total = 0

	g := new(errgroup.Group)
	g.Go(server)
	g.Go(client)
	return g.Wait()
}
