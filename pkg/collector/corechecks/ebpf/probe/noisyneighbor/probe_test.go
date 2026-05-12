// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package noisyneighbor

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/noisyneighbor/model"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestNoisyNeighborProbe(t *testing.T) {
	ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.RuntimeCompiled, ebpftest.CORE}, "", func(t *testing.T) {
		kv, err := kernel.HostVersion()
		if err != nil {
			t.Skipf("could not detect kernel version: %v", err)
		}
		if kv < minimumKernelVersion {
			t.Skipf("kernel version %v is below minimum %v", kv, minimumKernelVersion)
		}

		// TODO: Fedora 38 CI runners lack the perf_event_paranoid settings
		// needed for HW PMU events. Re-enable once the runner image is fixed.
		if strings.Contains(os.Getenv("CI_JOB_NAME"), "fedora_38") {
			t.Skipf("PMU events unavailable on %s", os.Getenv("CI_JOB_NAME"))
		}

		cfg := ebpf.NewConfig()
		probe, err := NewProbe(cfg)
		if err != nil {
			t.Fatalf("NewProbe(BPFDebug=%v) error = %v, want nil", cfg.BPFDebug, err)
		}
		t.Cleanup(probe.Close)

		var (
			observed model.Stats
			lastErr  error
			lastLen  int
		)
		deadline := time.After(10 * time.Second)
		tick := time.NewTicker(500 * time.Millisecond)
		defer tick.Stop()
		var found bool
		for !found {
			select {
			case <-deadline:
				t.Fatalf("Probe.GetAndFlush() produced no active cgroup within 10s; last error=%v, last record count=%d", lastErr, lastLen)
			case <-tick.C:
				stats, err := probe.GetAndFlush(context.Background())
				lastErr = err
				lastLen = len(stats)
				if err != nil {
					continue
				}
				for _, r := range stats {
					if r.EventCount > 0 && r.UniquePidCount > 0 {
						observed = r
						found = true
						break
					}
				}
			}
		}

		// EventCount and UniquePidCount are non-zero on any host where any
		// process is scheduled — they don't depend on HW PMU support.
		if observed.EventCount == 0 {
			t.Errorf("Probe.GetAndFlush() Stats{CgroupID:%d}.EventCount = 0, want > 0 (full record: %+v)", observed.CgroupID, observed)
		}
		if observed.UniquePidCount == 0 {
			t.Errorf("Probe.GetAndFlush() Stats{CgroupID:%d}.UniquePidCount = 0, want > 0 (full record: %+v)", observed.CgroupID, observed)
		}
		if observed.SumLatenciesNs == 0 {
			t.Errorf("Probe.GetAndFlush() Stats{CgroupID:%d}.SumLatenciesNs = 0, want > 0 (full record: %+v)", observed.CgroupID, observed)
		}
		if observed.WakeupCount == 0 {
			t.Errorf("Probe.GetAndFlush() Stats{CgroupID:%d}.WakeupCount = 0, want > 0 (full record: %+v)", observed.CgroupID, observed)
		}
	})
}

func TestAggregatePerCPU(t *testing.T) {
	tests := []struct {
		name string
		in   []ebpfCgroupAggStats
		want model.Stats
	}{
		{
			name: "empty",
			in:   nil,
			want: model.Stats{CgroupID: 42},
		},
		{
			name: "single CPU sums into output",
			in: []ebpfCgroupAggStats{
				{
					Sum_latencies_ns:     100,
					Event_count:          5,
					Preemption_count:     2,
					Pid_count:            3,
					Sum_cycles:           1000,
					Sum_instructions:     2000,
					Sum_llc_misses:       10,
					Sum_itlb_misses:      4,
					Sum_softirq_ns:       7,
					Block_io_requests:    1,
					Sum_branch_misses:    20,
					Sum_cpu_migrations:   8,
					Wakeup_count:         9,
					Sum_cache_references: 50,
				},
			},
			want: model.Stats{
				CgroupID:           42,
				SumLatenciesNs:     100,
				EventCount:         5,
				PreemptionCount:    2,
				UniquePidCount:     3,
				SumCycles:          1000,
				SumInstructions:    2000,
				SumLLCMisses:       10,
				SumITLBMisses:      4,
				SumSoftirqNs:       7,
				BlockIORequests:    1,
				SumBranchMisses:    20,
				SumCPUMigrations:   8,
				WakeupCount:        9,
				SumCacheReferences: 50,
			},
		},
		{
			name: "pid_count is max across CPUs, other counters are summed",
			in: []ebpfCgroupAggStats{
				{Sum_latencies_ns: 50, Event_count: 1, Pid_count: 5},
				{Sum_latencies_ns: 50, Event_count: 2, Pid_count: 3},
				{Sum_latencies_ns: 50, Event_count: 4, Pid_count: 7},
			},
			want: model.Stats{
				CgroupID:       42,
				SumLatenciesNs: 150,
				EventCount:     7,
				UniquePidCount: 7,
			},
		},
		{
			name: "multi-CPU sums all PMU counters",
			in: []ebpfCgroupAggStats{
				{Sum_cycles: 100, Sum_instructions: 50, Sum_branch_misses: 3, Sum_llc_misses: 10, Sum_itlb_misses: 2, Sum_cpu_migrations: 1, Sum_cache_references: 30, Sum_softirq_ns: 5, Block_io_requests: 2, Wakeup_count: 4, Event_count: 1, Pid_count: 2},
				{Sum_cycles: 200, Sum_instructions: 75, Sum_branch_misses: 4, Sum_llc_misses: 15, Sum_itlb_misses: 3, Sum_cpu_migrations: 2, Sum_cache_references: 40, Sum_softirq_ns: 6, Block_io_requests: 3, Wakeup_count: 5, Event_count: 1, Pid_count: 2},
			},
			want: model.Stats{
				CgroupID:           42,
				SumCycles:          300,
				SumInstructions:    125,
				SumBranchMisses:    7,
				SumLLCMisses:       25,
				SumITLBMisses:      5,
				SumCPUMigrations:   3,
				SumCacheReferences: 70,
				SumSoftirqNs:       11,
				BlockIORequests:    5,
				WakeupCount:        9,
				EventCount:         2,
				UniquePidCount:     2,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got model.Stats
			aggregatePerCPU(&got, 42, tt.in)
			if !assert.ObjectsAreEqual(tt.want, got) {
				t.Errorf("aggregatePerCPU(42, %+v):\n got: %+v\nwant: %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestHasActivity(t *testing.T) {
	tests := []struct {
		name string
		in   model.Stats
		want bool
	}{
		{"zero", model.Stats{}, false},
		{"events only", model.Stats{EventCount: 1}, true},
		{"softirq only", model.Stats{SumSoftirqNs: 1}, true},
		{"block io only", model.Stats{BlockIORequests: 1}, true},
		{"wakeups only", model.Stats{WakeupCount: 1}, true},
		{"latencies without events does not count", model.Stats{SumLatenciesNs: 999}, false},
		{"cycles without events does not count", model.Stats{SumCycles: 999}, false},
		{"preemption without events does not count", model.Stats{PreemptionCount: 5}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasActivity(&tt.in); got != tt.want {
				t.Errorf("hasActivity(%+v) = %t, want %t", tt.in, got, tt.want)
			}
		})
	}
}
