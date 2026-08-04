// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux_bpf

package numamonitoring

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var kv = kernel.MustHostVersion()

func TestSchedulerRuntimeAggregation(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.CORE, "", func(t *testing.T) {
		if kv < minimumKernelVersion {
			t.Skipf("Kernel version %v is not supported by the NUMA monitoring probe", kv)
		}

		probe, err := NewProbe(ebpf.NewConfig(), 1)
		require.NoError(t, err)
		t.Cleanup(probe.Close)
		require.Eventually(t, func() bool {
			stats := probe.flushRuntime()
			for _, nodes := range stats {
				for _, runtimeNS := range nodes {
					if runtimeNS > 0 {
						return true
					}
				}
			}
			return false
		}, 10*time.Second, 250*time.Millisecond)
	})
}
