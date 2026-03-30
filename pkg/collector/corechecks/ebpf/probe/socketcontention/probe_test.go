// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux_bpf

package socketcontention

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestSocketContentionProbe(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.CORE, "", func(t *testing.T) {
		kv := kernel.MustHostVersion()
		if kv < minimumKernelVersion {
			t.Skipf("Kernel version %v is not supported by the socket contention probe", kv)
		}
		if !contentionTracepointsSupported() {
			t.Skip("lock contention tracepoints are not available on this kernel")
		}

		probe, err := NewProbe(testConfig())
		require.NoError(t, err)
		t.Cleanup(probe.Close)

		require.Eventually(t, func() bool {
			runSocketWorkload(t)

			stats := probe.GetAndFlush()
			if stats.Count == 0 || stats.TotalTimeNS == 0 || stats.MaxTimeNS == 0 || stats.MinTimeNS == 0 {
				return false
			}

			flushed := probe.GetAndFlush()
			return flushed.Count == 0 && flushed.TotalTimeNS == 0
		}, 10*time.Second, 250*time.Millisecond, "failed to observe contention stats")
	})
}

func testConfig() *ebpf.Config {
	return ebpf.NewConfig()
}

func runSocketWorkload(t *testing.T) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	var acceptWG sync.WaitGroup
	acceptWG.Add(1)
	go func() {
		defer acceptWG.Done()
		for i := 0; i < 32; i++ {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = io.Copy(io.Discard, conn)
			_ = conn.Close()
		}
	}()

	var clientWG sync.WaitGroup
	for i := 0; i < 32; i++ {
		clientWG.Add(1)
		go func() {
			defer clientWG.Done()
			conn, err := net.Dial("tcp", ln.Addr().String())
			require.NoError(t, err)
			_, _ = conn.Write(make([]byte, 4096))
			_ = conn.Close()
		}()
	}

	clientWG.Wait()
	_ = ln.Close()
	acceptWG.Wait()
}
