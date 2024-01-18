// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpfcheck

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
	"golang.org/x/sys/unix"

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

		var result model.EBPFMapStats
		require.Eventually(t, func() bool {
			stats := probe.GetAndFlush()
			for _, s := range stats.Maps {
				if s.Type == ebpf.PerfEventArray && s.Name == "ebpf_test_perf" {
					result = s
					return true
				}
			}
			for _, s := range stats.Maps {
				t.Logf("%+v", s)
			}
			return false
		}, 5*time.Second, 500*time.Millisecond, "failed to find perf buffer map")

		// use number of CPUs from result
		// this isn't as strict, but only way to ensure same number of CPUs is used
		onlineCPUs := uint64(result.NumCPUs)

		// 4 is value size, 1 extra page for metadata
		valueSize := uint64(roundUpPow2(4, 8))
		expected := (onlineCPUs * uint64(pageSize) * uint64(numPages+1)) + nrcpus*valueSize + sizeofBpfArray
		if result.MaxSize != expected {
			t.Fatalf("expected perf buffer size %d got %d", expected, result.MaxSize)
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

		mapTypes := []*ebpf.MapSpec{
			{Type: ebpf.Array, KeySize: 4},
			{Type: ebpf.PerCPUArray, KeySize: 4},
			{Type: ebpf.Hash, KeySize: keySize},
			{Type: ebpf.LRUHash, KeySize: keySize},
			{Type: ebpf.LRUCPUHash, KeySize: keySize},
			{Type: ebpf.PerCPUHash, KeySize: keySize},
			{Type: ebpf.LPMTrie, KeySize: 8, ValueSize: 8, Flags: unix.BPF_F_NO_PREALLOC},
		}

		// create all the maps
		ids := make(map[ebpf.MapType]ebpf.MapID)
		for _, mt := range mapTypes {
			if mt.ValueSize == 0 {
				mt.ValueSize = valueSize
			}
			if mt.MaxEntries == 0 {
				mt.MaxEntries = maxEntries
			}
			mt.Name = fmt.Sprintf("et_%s", mt.Type)
			testMap, err := ebpf.NewMap(mt)
			require.NoError(t, err, mt.Type)
			t.Cleanup(func() { _ = testMap.Close() })
			info, err := testMap.Info()
			require.NoError(t, err, mt.Type)
			ids[mt.Type], _ = info.ID()
		}

		// wait until we have stats about all maps
		var result []model.EBPFMapStats
		require.Eventually(t, func() bool {
			stats := probe.GetAndFlush()
			for typ, id := range ids {
				if !slices.ContainsFunc(stats.Maps, func(stats model.EBPFMapStats) bool {
					return stats.ID == uint32(id)
				}) {
					t.Logf("missing type=%s id=%d", typ.String(), id)
					return false
				}
			}
			result = stats.Maps
			return true
		}, 5*time.Second, 500*time.Millisecond, "failed to find all maps")

		// assert max size is at least the naive map size
		for _, mt := range mapTypes {
			idx := slices.IndexFunc(result, func(stats model.EBPFMapStats) bool {
				return stats.ID == uint32(ids[mt.Type])
			})
			typStats := result[idx]

			ks := uint64(mt.KeySize)
			switch mt.Type {
			case ebpf.Array, ebpf.PerCPUArray:
				// array types don't use space for the indexes
				ks = 1
			}

			minSize := uint64(0)
			if isPerCPU(mt.Type) {
				// hash of key -> ~per-cpu array
				minSize = (ks * uint64(mt.MaxEntries)) + (uint64(mt.ValueSize) * uint64(mt.MaxEntries) * nrcpus)
			} else {
				minSize = (ks + uint64(mt.ValueSize)) * uint64(mt.MaxEntries)
			}
			t.Logf("type: %s min: %d val: %d", mt.Type, minSize, typStats.MaxSize)
			assert.GreaterOrEqual(t, typStats.MaxSize, minSize, "map type: %s", mt.Type)
		}
	})
}

func testConfig() *ddebpf.Config {
	cfg := ddebpf.NewConfig()
	return cfg
}

func TestHashMapNumberOfEntries(t *testing.T) {
	ebpftest.RequireKernelVersion(t, minimumKernelVersion)
	maxEntries := uint32(1000)
	filledEntries := uint32(500)

	m, err := ebpf.NewMap(&ebpf.MapSpec{
		Type:       ebpf.Hash,
		MaxEntries: uint32(maxEntries),
		KeySize:    4,
		ValueSize:  4,
	})
	require.NoError(t, err)

	for i := uint32(0); i < filledEntries; i++ {
		require.NoError(t, m.Put(&i, &i))
	}

	require.Equal(t, int64(filledEntries), hashMapNumberOfEntries(m))
}

func TestHashMapNumberOfEntriesNoExtraAllocations(t *testing.T) {
	ebpftest.RequireKernelVersion(t, minimumKernelVersion)
	entriesToTest := []uint32{10, 100, 1000, 10000}
	maxAllocs := 6.0

	for _, maxEntries := range entriesToTest {
		t.Run(fmt.Sprintf("%dMaxEntries", maxEntries), func(t *testing.T) {
			maxEntries := uint32(1000)
			filledEntries := uint32(500)

			m, err := ebpf.NewMap(&ebpf.MapSpec{
				Type:       ebpf.Hash,
				MaxEntries: uint32(maxEntries),
				KeySize:    4,
				ValueSize:  4,
			})
			require.NoError(t, err)

			for i := uint32(0); i < filledEntries; i++ {
				require.NoError(t, m.Put(&i, &i))
			}

			allocs := testing.AllocsPerRun(10, func() {
				hashMapNumberOfEntries(m)
			})
			require.LessOrEqual(t, allocs, maxAllocs)
		})
	}
}

func TestLRUHashMapNumberOfEntries(t *testing.T) {
	ebpftest.RequireKernelVersion(t, minimumKernelVersion)
	maxEntries := uint32(1000)
	filledEntries := uint32(500)

	m, err := ebpf.NewMap(&ebpf.MapSpec{
		Type:       ebpf.LRUHash,
		MaxEntries: uint32(maxEntries),
		KeySize:    4,
		ValueSize:  4,
	})
	require.NoError(t, err)

	for i := uint32(0); i < filledEntries; i++ {
		require.NoError(t, m.Put(&i, &i))
	}

	require.Equal(t, int64(filledEntries), hashMapNumberOfEntries(m))
}
