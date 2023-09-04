// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpfcheck

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"

	"github.com/DataDog/gopsutil/process"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/stretchr/testify/require"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestEBPFPerfBufferLength(t *testing.T) {
	err := rlimit.RemoveMemlock()
	require.NoError(t, err)

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
					exp := mm.Rss * 1024
					if exp != cpub.RSS {
						t.Fatalf("expected RSS %d got %d", exp, cpub.RSS)
					}
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

func BenchmarkMatchProcessRSS(b *testing.B) {
	pid := os.Getpid()
	addrs := []uintptr{}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := matchProcessRSS(pid, addrs)
		require.NoError(b, err)
	}
}

func BenchmarkMatchRSS(b *testing.B) {
	data, err := os.ReadFile("./testdata/smaps.out")
	require.NoError(b, err)
	rdr := bytes.NewReader(data)
	addrs := []uintptr{}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := rdr.Seek(0, io.SeekStart)
		require.NoError(b, err)
		_, err = matchRSS(rdr, addrs)
		require.NoError(b, err)
	}
}
