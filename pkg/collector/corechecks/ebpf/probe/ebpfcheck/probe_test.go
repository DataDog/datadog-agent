// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpfcheck

import (
	"os"
	"testing"
	"time"

	"github.com/DataDog/gopsutil/process"
	"github.com/cihub/seelog"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/perf"
	"github.com/stretchr/testify/require"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestMain(m *testing.M) {
	logLevel := os.Getenv("DD_LOG_LEVEL")
	if logLevel == "" {
		logLevel = "warn"
	}
	log.SetupLogger(seelog.Default, logLevel)
	os.Exit(m.Run())
}

func TestEBPFPerfBufferLength(t *testing.T) {
	ebpftest.RequireKernelVersion(t, minimumKernelVersion)
	ebpftest.TestBuildMode(t, ebpftest.CORE, "", func(t *testing.T) {
		cpus, err := kernel.PossibleCPUs()
		require.NoError(t, err)
		nrcpus := uint64(cpus)

		online, err := kernel.OnlineCPUs()
		require.NoError(t, err)
		onlineCPUs := uint64(len(online))

		cfg := testConfig()

		probe, err := NewEBPFProbe(cfg)
		require.NoError(t, err)
		t.Cleanup(probe.Close)

		pageSize := os.Getpagesize()
		numPages := 8 // must be power of two for test to pass, because it is rounded up internally

		pe := &ebpf.MapSpec{Name: "ebpf_test_perf", Type: ebpf.PerfEventArray}
		peMap, err := ebpf.NewMap(pe)
		require.NoError(t, err)
		t.Cleanup(func() { _ = peMap.Close() })

		rdr, err := perf.NewReader(peMap, numPages*pageSize)
		require.NoError(t, err)
		t.Cleanup(func() { _ = rdr.Close() })

		var result EBPFPerfBufferStats
		require.Eventually(t, func() bool {
			stats := probe.GetAndFlush()
			for _, s := range stats.PerfBuffers {
				if s.Name == "ebpf_test_perf" {
					result = s
					return true
				}
			}
			for _, s := range stats.PerfBuffers {
				t.Logf("%+v", s)
			}
			return false
		}, 5*time.Second, 500*time.Millisecond, "failed to find perf buffer map")

		// 4 is value size, 1 extra page for metadata
		expected := (onlineCPUs * uint64(pageSize) * uint64(numPages+1)) + nrcpus*4 + sizeofBpfArray
		if result.MaxSize != expected {
			t.Fatalf("expected perf buffer size %d got %d", expected, result.MaxSize)
		}

		proc, err := process.NewProcess(int32(os.Getpid()))
		require.NoError(t, err)
		mmaps, err := proc.MemoryMaps(false)
		require.NoError(t, err)

		for _, cpub := range result.CPUBuffers {
			for _, mm := range *mmaps {
				if mm.StartAddr == cpub.Addr {
					t.Log(mm)
					break
				}
			}
		}
	})
}

func testConfig() *ddebpf.Config {
	cfg := ddebpf.NewConfig()
	return cfg
}
