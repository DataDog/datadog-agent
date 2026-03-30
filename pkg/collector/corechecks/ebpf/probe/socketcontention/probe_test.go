// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux_bpf

package socketcontention

import (
	"net"
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

		probe, err := NewProbe(testConfig())
		require.NoError(t, err)
		t.Cleanup(probe.Close)

		require.Eventually(t, func() bool {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			_ = ln.Close()

			stats := probe.GetAndFlush()
			return stats.SocketInits > 0
		}, 5*time.Second, 200*time.Millisecond, "failed to observe socket init stats")
	})
}

func testConfig() *ebpf.Config {
	return ebpf.NewConfig()
}
