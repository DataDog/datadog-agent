// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpfcheck

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/DataDog/gopsutil/process"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/ebpfcheck/model"
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

		probe, err := NewProbe(cfg)
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

		var result model.EBPFPerfBufferStats
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
		valueSize := uint64(roundUpPow2(4, 8))
		expected := (onlineCPUs * uint64(pageSize) * uint64(numPages+1)) + nrcpus*valueSize + sizeofBpfArray
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

func TestMinMapSize(t *testing.T) {
	ebpftest.RequireKernelVersion(t, minimumKernelVersion)
	err := rlimit.RemoveMemlock()
	require.NoError(t, err)

	cpus, err := kernel.PossibleCPUs()
	require.NoError(t, err)
	nrcpus := uint64(cpus)

	ebpftest.TestBuildMode(t, ebpftest.CORE, "", func(t *testing.T) {
		cfg := testConfig()
		const keySize, valueSize, maxEntries = 50, 150, 1000

		probe, err := NewProbe(cfg)
		require.NoError(t, err)
		t.Cleanup(probe.Close)

		mapTypes := []struct {
			typ     ebpf.MapType
			keySize uint32
		}{
			{typ: ebpf.Array, keySize: 4},
			{typ: ebpf.PerCPUArray, keySize: 4},
			{typ: ebpf.Hash, keySize: keySize},
			{typ: ebpf.LRUHash, keySize: keySize},
			{typ: ebpf.LRUCPUHash, keySize: keySize},
			{typ: ebpf.PerCPUHash, keySize: keySize},
		}

		// create all the maps
		ids := make(map[ebpf.MapType]ebpf.MapID)
		for _, mt := range mapTypes {
			testMapSpec := &ebpf.MapSpec{Name: fmt.Sprintf("et_%s", mt.typ), Type: mt.typ, KeySize: mt.keySize, ValueSize: valueSize, MaxEntries: maxEntries}
			testMap, err := ebpf.NewMap(testMapSpec)
			require.NoError(t, err, mt.typ)
			t.Cleanup(func() { _ = testMap.Close() })
			info, err := testMap.Info()
			require.NoError(t, err, mt.typ)
			ids[mt.typ], _ = info.ID()
		}

		// wait until we have stats about all maps
		var result []model.EBPFMapStats
		require.Eventually(t, func() bool {
			stats := probe.GetAndFlush()
			t.Logf("%+v", stats.Maps)
			for _, id := range ids {
				if !slices.ContainsFunc(stats.Maps, func(stats model.EBPFMapStats) bool {
					return stats.ID == uint32(id)
				}) {
					return false
				}
			}
			result = stats.Maps
			return true
		}, 5*time.Second, 500*time.Millisecond, "failed to find all maps")

		// assert max size is at least the naive map size
		for _, mt := range mapTypes {
			idx := slices.IndexFunc(result, func(stats model.EBPFMapStats) bool {
				return stats.ID == uint32(ids[mt.typ])
			})
			typStats := result[idx]

			ks := uint64(keySize)
			switch mt.typ {
			case ebpf.Array, ebpf.PerCPUArray:
				// array types don't use space for the indexes
				ks = 1
			}

			minSize := uint64(0)
			if isPerCPU(mt.typ) {
				// hash of key -> ~per-cpu array
				minSize = (ks * uint64(maxEntries)) + (uint64(valueSize) * uint64(maxEntries) * nrcpus)
			} else {
				minSize = (ks + valueSize) * maxEntries
			}
			t.Logf("typ: %s min: %d val: %d", mt.typ, minSize, typStats.MaxSize)
			assert.GreaterOrEqual(t, typStats.MaxSize, minSize, "map type: %s", mt.typ.String())
		}
	})
}

func testConfig() *ddebpf.Config {
	cfg := ddebpf.NewConfig()
	return cfg
}

func BenchmarkMatchProcessRSS(b *testing.B) {
	pid := os.Getpid()
	var addrs []uintptr

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
	var addrs []uintptr

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := rdr.Seek(0, io.SeekStart)
		require.NoError(b, err)
		_, err = matchRSS(rdr, addrs)
		require.NoError(b, err)
	}
}
