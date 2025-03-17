// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package noisyneighbor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var kv = kernel.MustHostVersion()

func TestNoisyNeighborProbe(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.CORE, "", func(t *testing.T) {
		if kv < minimumKernelVersion {
			t.Skipf("Kernel version %v is not supported by the Noisy Neighbor probe", kv)
		}

		cfg := testConfig()
		probe, err := NewProbe(cfg)
		require.NoError(t, err)
		t.Cleanup(probe.Close)

		require.Eventually(t, func() bool {
			for _, r := range probe.GetAndFlush() {
				if r.Name != "" {
					return true
				}
			}
			return false
		}, 10*time.Second, 500*time.Millisecond, "failed to find an noisy neighbor process")
	})
}

func testConfig() *ebpf.Config {
	cfg := ebpf.NewConfig()
	return cfg
}
