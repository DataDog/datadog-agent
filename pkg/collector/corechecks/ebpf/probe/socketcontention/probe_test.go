// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux_bpf

package socketcontention

import (
	"io"
	"net"
	"path/filepath"
	"runtime"
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

		probe, err := NewProbe(testConfig(t))
		require.NoError(t, err)
		t.Cleanup(probe.Close)

		baselineIdentities, err := probe.DebugListLockIdentities()
		require.NoError(t, err)
		baselineLockAddrs := make(map[uint64]struct{}, len(baselineIdentities))
		for _, entry := range baselineIdentities {
			baselineLockAddrs[entry.LockAddr] = struct{}{}
		}

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer ln.Close()

		accepted := make(chan net.Conn, 1)
		go func() {
			conn, acceptErr := ln.Accept()
			if acceptErr == nil {
				accepted <- conn
			}
		}()

		clientConn, err := net.Dial("tcp", ln.Addr().String())
		require.NoError(t, err)
		serverConn := <-accepted

		var registeredLockAddrs map[uint64]struct{}
		require.Eventually(t, func() bool {
			identities, lookupErr := probe.DebugListLockIdentities()
			require.NoError(t, lookupErr)
			candidates := make(map[uint64]struct{})
			for _, entry := range identities {
				if _, ok := baselineLockAddrs[entry.LockAddr]; ok {
					continue
				}
				if entry.SocketType != "stream" || entry.Protocol != "tcp" {
					continue
				}
				candidates[entry.LockAddr] = struct{}{}
			}
			if len(candidates) > 0 {
				registeredLockAddrs = candidates
				return true
			}
			return false
		}, 5*time.Second, 100*time.Millisecond, "failed to observe socket registration")
		require.NotEmpty(t, registeredLockAddrs)

		_ = clientConn.Close()
		_ = serverConn.Close()
		_ = ln.Close()

		require.Eventually(t, func() bool {
			identities, lookupErr := probe.DebugListLockIdentities()
			require.NoError(t, lookupErr)
			current := make(map[uint64]struct{}, len(identities))
			for _, entry := range identities {
				current[entry.LockAddr] = struct{}{}
			}
			for lockAddr := range registeredLockAddrs {
				if _, ok := current[lockAddr]; ok {
					return false
				}
			}
			return true
		}, 5*time.Second, 100*time.Millisecond, "failed to observe socket cleanup")

		// Unknown buckets are filtered out now, so this integration test focuses on
		// verifying lifecycle registration and cleanup. Aggregation/formatting is
		// covered by the probe unit tests.
		runSocketWorkload(t)
		require.Empty(t, probe.GetAndFlush())
	})
}

func testConfig(t *testing.T) *ebpf.Config {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "failed to determine test file path")

	cfg := ebpf.NewConfig()
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../../.."))
	cfg.BPFDir = filepath.Join(repoRoot, "pkg/ebpf/bytecode/build", "arm64")
	return cfg
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
