// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package noisyneighbor is the system-probe side of the Noisy Neighbor check.
package noisyneighbor

import (
	"os"
	"strings"
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

		if strings.Contains(os.Getenv("CI_JOB_NAME"), "fedora_38") {
			t.Skipf("Noisy Neighbor probe is not supported on this environment: %s", os.Getenv("CI_JOB_NAME"))
		}

		t.Logf("testing on %s", os.Getenv("CI_JOB_NAME"))

		cfg := testConfig()
		probe, err := NewProbe(cfg, Config{PMUMetrics: allPMUMetricsEnabled()})
		require.NoError(t, err)
		t.Cleanup(probe.Close)

		require.Eventually(t, func() bool {
			for _, r := range probe.GetAndFlush() {
				if r.EventCount > 0 || r.PreemptionCount > 0 {
					return true
				}
			}
			return false
		}, 10*time.Second, 500*time.Millisecond, "failed to get noisy neighbor stats")
	})
}

func testConfig() *ebpf.Config {
	cfg := ebpf.NewConfig()
	return cfg
}

// allPMUMetricsEnabled returns the PMU-metrics map that NewProbe expects,
// with every counter enabled — the default tested configuration.
func allPMUMetricsEnabled() map[string]bool {
	return map[string]bool{
		"cycles_pmu":           true,
		"instructions_pmu":     true,
		"cache_misses_pmu":     true,
		"cache_references_pmu": true,
		"itlb_misses_pmu":      true,
		"branch_misses_pmu":    true,
		"cpu_migrations_pmu":   true,
	}
}
