// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package tcpqueuelength

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/tcpqueuelength/model"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
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
		out, err := runtime.TcpQueueLength.Compile(cfg, []string{"-g"})
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

		assert.EventuallyWithT(t, func(c *assert.CollectT) {
			beforeStats := extractGlobalStats(t, tcpTracer)
			if beforeStats.ReadBufferMaxUsage > 10 {
				c.Errorf("max usage of read buffer is too big before the stress test: %d > 10", beforeStats.ReadBufferMaxUsage)
			}
		}, 3*time.Second, 1*time.Second)

		err = runTCPLoadTest()
		require.NoError(t, err)

		var maxReadUsage uint32
		assert.EventuallyWithT(t, func(c *assert.CollectT) {
			afterStats := extractGlobalStats(t, tcpTracer)
			if afterStats.ReadBufferMaxUsage > maxReadUsage {
				maxReadUsage = afterStats.ReadBufferMaxUsage
			}
			assert.GreaterOrEqual(c, maxReadUsage, uint32(1000), "max usage of read buffer is too low after the stress test")
		}, 5*time.Second, 500*time.Millisecond)
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
// The idea here is to setup a server and a client with a tiny RECV buffer,
// then send enough data to fill the buffer. The eBPF probe captures the
// buffer usage ratio (per-mille) during tcp_recvmsg calls.

var tcpTestAddr = &net.TCPAddr{
	Port: 25568,
}

const msgLen = 10000

func runTCPLoadTest() error {
	serverReady := make(chan struct{})
	bufferConfigured := make(chan struct{})

	g, ctx := errgroup.WithContext(context.Background())

	// Server goroutine: listen, accept, configure a tiny receive buffer, then read.
	g.Go(func() error {
		listener, err := net.ListenTCP("tcp", tcpTestAddr)
		if err != nil {
			return err
		}
		defer listener.Close()

		close(serverReady)

		conn, err := listener.AcceptTCP()
		if err != nil {
			return err
		}
		defer conn.Close()

		if err := conn.SetReadBuffer(2); err != nil {
			return fmt.Errorf("SetReadBuffer: %w", err)
		}
		close(bufferConfigured)

		// Read all data in small chunks until the zero-byte terminator.
		for {
			buf := make([]byte, 10)
			count, err := conn.Read(buf)
			if err != nil {
				return err
			}

			for i := 0; i < count; i++ {
				if buf[i] == 0 {
					return nil
				}
			}
		}
	})

	// Client goroutine: connect, wait for the server to configure its small
	// buffer, then send data.
	g.Go(func() error {
		select {
		case <-serverReady:
		case <-ctx.Done():
			return ctx.Err()
		}

		conn, err := net.DialTCP("tcp", nil, tcpTestAddr)
		if err != nil {
			return err
		}
		defer conn.Close()

		// Wait until the server has accepted the connection and configured
		// a small receive buffer. This ensures that when we send data, the
		// kernel sk_rcvbuf is already small, so the eBPF probe will see a
		// high buffer usage ratio.
		select {
		case <-bufferConfigured:
		case <-ctx.Done():
			return ctx.Err()
		}

		msg := make([]byte, msgLen)
		for i := 0; i < msgLen-1; i++ {
			msg[i] = 4
		}
		msg[msgLen-1] = 0

		_, err = conn.Write(msg)
		return err
	})

	return g.Wait()
}
