// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpfcheck

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"slices"
	"testing"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/ebpfcheck/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestEBPFPerfBufferLength(t *testing.T) {
	ebpftest.FailLogLevel(t, "trace")

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
	ebpftest.TestBuildMode(t, ebpftest.CORE, "", func(t *testing.T) {
		maxEntries := uint32(50)

		testWithEntryCount := func(t *testing.T, mapType ebpf.MapType, filledEntries uint32) {
			var innerMapSpec *ebpf.MapSpec
			buffers := entryCountBuffers{
				keysBufferSizeLimit:   0, // No limit
				valuesBufferSizeLimit: 0, // No limit
			}
			if mapType == ebpf.HashOfMaps {
				innerMapSpec = &ebpf.MapSpec{
					Type:       ebpf.Hash,
					MaxEntries: uint32(20),
					KeySize:    4,
					ValueSize:  4,
				}
			}

			m, err := ebpf.NewMap(&ebpf.MapSpec{
				Type:       mapType,
				MaxEntries: uint32(maxEntries),
				KeySize:    4,
				ValueSize:  4,
				InnerMap:   innerMapSpec,
			})
			require.NoError(t, err)
			t.Cleanup(func() { _ = m.Close() })

			for i := uint32(0); i < filledEntries; i++ {
				if mapType == ebpf.HashOfMaps {
					innerMap, err := ebpf.NewMap(innerMapSpec)
					require.NoError(t, err)
					t.Cleanup(func() { _ = innerMap.Close() })
					require.NoError(t, m.Put(&i, innerMap))
				} else {
					require.NoError(t, m.Put(&i, &i))
				}
			}

			if maps.BatchAPISupported() && mapType != ebpf.HashOfMaps {
				t.Run("BatchAPI", func(t *testing.T) {
					num, err := hashMapNumberOfEntriesWithBatch(m, &buffers, 1)
					require.NoError(t, err)
					require.Equal(t, int64(filledEntries), num)
				})
			}

			t.Run("Iteration", func(t *testing.T) {
				num, err := hashMapNumberOfEntriesWithIteration(m, &buffers, 1)
				require.NoError(t, err)
				require.Equal(t, int64(filledEntries), num)
			})

			// Test the complete function just in case
			require.Equal(t, int64(filledEntries), hashMapNumberOfEntries(m, &buffers, 1))
		}

		mapTypes := []ebpf.MapType{ebpf.Hash, ebpf.LRUHash, ebpf.HashOfMaps}
		for _, mapType := range mapTypes {
			t.Run(mapType.String(), func(t *testing.T) {
				t.Run("EmptyMap", func(t *testing.T) { testWithEntryCount(t, mapType, 0) })
				t.Run("HalfFullMap", func(t *testing.T) { testWithEntryCount(t, mapType, maxEntries/2) })

				if mapType != ebpf.LRUHash { // LRUHash starts vacating entries even when it's not 100% full, cannot test this case
					t.Run("FullMap", func(t *testing.T) { testWithEntryCount(t, mapType, maxEntries) })
				}
			})
		}
	})
}

func TestHashMapNumberOfEntriesNoExtraAllocations(t *testing.T) {
	ebpftest.RequireKernelVersion(t, minimumKernelVersion)
	minBatchSize := uint32(8) // Ensure all numbers are divisible by 8 so that we can have whole numbers in the MultipleBatch case
	entriesToTest := []uint32{minBatchSize * 5, minBatchSize * 15, minBatchSize * 125, minBatchSize * 1250}

	for _, maxEntries := range entriesToTest {
		t.Run(fmt.Sprintf("%dMaxEntries", maxEntries), func(t *testing.T) {
			filledEntries := maxEntries / 2
			buffers := entryCountBuffers{
				keysBufferSizeLimit:   0, // No limit
				valuesBufferSizeLimit: 0, // No limit
			}

			m, err := ebpf.NewMap(&ebpf.MapSpec{
				Type:       ebpf.Hash,
				MaxEntries: uint32(maxEntries),
				KeySize:    4,
				ValueSize:  4,
			})
			require.NoError(t, err)
			t.Cleanup(func() { _ = m.Close() })
			buffers.tryEnsureSizeForFullBatch(m)

			for i := uint32(0); i < filledEntries; i++ {
				require.NoError(t, m.Put(&i, &i))
			}

			t.Run("Iteration", func(t *testing.T) {
				allocs := testing.AllocsPerRun(10, func() {
					hashMapNumberOfEntriesWithIteration(m, &buffers, 1)
				})
				require.LessOrEqual(t, allocs, 2.0) // Allocations come from the ErrKeyNotExist (which is the end-of-iteration marker) in cilium/ebpf
			})

			if maps.BatchAPISupported() {
				t.Run("Batch", func(t *testing.T) {
					allocs := testing.AllocsPerRun(10, func() {
						hashMapNumberOfEntriesWithBatch(m, &buffers, 1)
					})
					require.LessOrEqual(t, allocs, 0.0)
				})

				t.Run("MultipleBatch", func(t *testing.T) {
					limitedBuffers := entryCountBuffers{
						keysBufferSizeLimit:   m.KeySize() * filledEntries / 4,
						valuesBufferSizeLimit: m.ValueSize() * filledEntries / 4,
					}
					limitedBuffers.tryEnsureSizeForFullBatch(m)
					limitedBuffers.prepareFirstBatchKeys(m)

					allocs := testing.AllocsPerRun(10, func() {
						hashMapNumberOfEntriesWithBatch(m, &limitedBuffers, 1)
					})
					require.LessOrEqual(t, allocs, 8.0) // Multiple batches mean we need to use a map to keep track of the keys, that causes allocations for the values
				})
			}

			t.Run("MainFunction", func(t *testing.T) {
				allocs := testing.AllocsPerRun(10, func() {
					hashMapNumberOfEntries(m, &buffers, 1)
				})
				require.LessOrEqual(t, allocs, 0.0)
			})
		})
	}
}

func TestHashMapNumberOfEntriesMapTypeSupport(t *testing.T) {
	ebpftest.RequireKernelVersion(t, minimumKernelVersion)
	ebpftest.TestBuildMode(t, ebpftest.CORE, "", func(t *testing.T) {
		maxEntries := uint32(1000)
		testMapType := func(t *testing.T, mapType ebpf.MapType, expectedReturn int64) {
			buffers := entryCountBuffers{
				keysBufferSizeLimit:   0, // No limit
				valuesBufferSizeLimit: 0, // No limit
			}
			var innerMap *ebpf.MapSpec
			if mapType == ebpf.HashOfMaps {
				innerMap = &ebpf.MapSpec{
					Type:       ebpf.Hash,
					MaxEntries: uint32(maxEntries),
					KeySize:    4,
					ValueSize:  4,
				}
			}

			m, err := ebpf.NewMap(&ebpf.MapSpec{
				Type:       mapType,
				MaxEntries: uint32(maxEntries),
				KeySize:    4,
				ValueSize:  4,
				InnerMap:   innerMap,
			})
			require.NoError(t, err)
			t.Cleanup(func() { _ = m.Close() })
			buffers.tryEnsureSizeForFullBatch(m)
			require.Equal(t, expectedReturn, hashMapNumberOfEntries(m, &buffers, 1))
		}

		// Test supported types first
		t.Run("Hash", func(t *testing.T) { testMapType(t, ebpf.Hash, 0) })
		t.Run("LRUHash", func(t *testing.T) { testMapType(t, ebpf.LRUHash, 0) })
		t.Run("HashOfMaps", func(t *testing.T) { testMapType(t, ebpf.HashOfMaps, 0) })

		// Now unsupported
		t.Run("PerCPUHash_Unsupported", func(t *testing.T) { testMapType(t, ebpf.PerCPUHash, -1) })
		t.Run("LRUCPUHash_Unsupported", func(t *testing.T) { testMapType(t, ebpf.LRUCPUHash, -1) })
	})
}

func TestHashMapNumberOfEntriesWithMultipleBatch(t *testing.T) {
	if !maps.BatchAPISupported() {
		t.Skip("Batch API not supported")
	}

	ebpftest.RequireKernelVersion(t, minimumKernelVersion)
	ebpftest.TestBuildMode(t, ebpftest.CORE, "", func(t *testing.T) {
		maxEntries := uint32(1000)
		filledEntries := uint32(200)
		keySize, valueSize := uint32(4), uint32(4)

		// Set the limits so that we need two batches
		buffers := entryCountBuffers{
			keysBufferSizeLimit:   100 * keySize,
			valuesBufferSizeLimit: 100 * valueSize,
		}

		m, err := ebpf.NewMap(&ebpf.MapSpec{
			Type:       ebpf.Hash,
			MaxEntries: uint32(maxEntries),
			KeySize:    keySize,
			ValueSize:  valueSize,
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = m.Close() })

		for i := uint32(0); i < filledEntries; i++ {
			require.NoError(t, m.Put(&i, &i))
		}

		num, err := hashMapNumberOfEntriesWithBatch(m, &buffers, 1)
		require.NoError(t, err)
		require.Equal(t, int64(filledEntries), num)
		require.Equal(t, uint32(len(buffers.keys)), buffers.keysBufferSizeLimit)
		require.Equal(t, uint32(len(buffers.values)), buffers.valuesBufferSizeLimit)
	})
}

func TestHashMapNumberOfEntriesNoMemoryCorruption(t *testing.T) {
	ebpftest.RequireKernelVersion(t, minimumKernelVersion)
	require.NoError(t, rlimit.RemoveMemlock())

	testInner := func(t *testing.T, mapType ebpf.MapType, filledEntries uint32, maxEntries uint32, keySize uint32, valueSize uint32, keysLimit uint32, valuesLimit uint32) {
		var innerMapSpec *ebpf.MapSpec
		buffers := entryCountBuffers{
			keysBufferSizeLimit:   keysLimit,
			valuesBufferSizeLimit: valuesLimit,
		}

		m, err := ebpf.NewMap(&ebpf.MapSpec{
			Type:       mapType,
			MaxEntries: maxEntries,
			KeySize:    keySize,
			ValueSize:  valueSize,
			InnerMap:   innerMapSpec,
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = m.Close() })

		keys := make([]byte, keySize)
		values := make([]byte, valueSize)

		for i := uint32(0); i < filledEntries; i++ {
			for j := uint32(0); j < valueSize; j++ {
				values[j] = byte(i)
			}
			// Build the keys in a way that makes them unique no matter the key size
			if keySize < 4 {
				for j := uint32(0); j < keySize; j++ {
					keys[j] = byte(i)
				}
			} else {
				binary.LittleEndian.PutUint32(keys, i)
			}

			if mapType == ebpf.HashOfMaps {
				innerMap, err := ebpf.NewMap(innerMapSpec)
				require.NoError(t, err)
				t.Cleanup(func() { _ = innerMap.Close() })
				require.NoError(t, m.Put(&keys, innerMap))
			} else {
				require.NoError(t, m.Put(&keys, &values))
			}
		}

		// Preallocate all the buffers
		buffers.tryEnsureSizeForFullBatch(m)
		buffers.prepareFirstBatchKeys(m)

		// Utility function to get a byte at a certain offset in a slice, in an unsafe manner
		unsafeIndex := func(slice []byte, index int) *byte {
			return (*byte)(unsafe.Pointer(uintptr(unsafe.Pointer(&slice[0])) + uintptr(index)))
		}

		// Grow the buffers more, add a magic number to that new part, we will verify if it's overwritten
		extraBytes := 10
		magicNumber := byte(0x42)
		addMargin := func(slice []byte) []byte {
			// Increase the capacity and not the length, the code uses length to calculate batch sizes so we need to keep it the same
			// in order to check if it's writing beyond that.
			newBuffer := make([]byte, len(slice), len(slice)+extraBytes)
			for i := 0; i < extraBytes; i++ {
				*unsafeIndex(newBuffer, len(slice)+i) = magicNumber
			}
			return newBuffer
		}

		validateMargin := func(t *testing.T) {
			for i := 0; i < extraBytes; i++ {
				require.Equal(t, magicNumber, *unsafeIndex(buffers.keys, len(buffers.keys)+i), "invalid magic byte at %d (%d bytes after the allocated keys buffer)", i+len(buffers.keys)-extraBytes, i)
				require.Equal(t, magicNumber, *unsafeIndex(buffers.values, len(buffers.values)+i), "invalid magic byte at %d (%d bytes after the allocated values buffer)", i+len(buffers.values)-extraBytes, i)
				require.Equal(t, magicNumber, *unsafeIndex(buffers.cursor, len(buffers.cursor)+i), "invalid magic byte at %d (%d bytes after the allocated cursor buffer)", i+len(buffers.cursor)-extraBytes, i)
			}
		}

		buffers.keys = addMargin(buffers.keys)
		buffers.values = addMargin(buffers.values)
		buffers.cursor = addMargin(buffers.cursor)
		validateMargin(t)

		if maps.BatchAPISupported() && mapType != ebpf.HashOfMaps {
			t.Run("BatchAPI", func(t *testing.T) {
				num, err := hashMapNumberOfEntriesWithBatch(m, &buffers, 3)
				require.NoError(t, err)
				require.Equal(t, int64(filledEntries), num)
				validateMargin(t)
			})
		}

		t.Run("Iteration", func(t *testing.T) {
			num, err := hashMapNumberOfEntriesWithIteration(m, &buffers, 3)
			require.NoError(t, err)
			require.Equal(t, int64(filledEntries), num)
			validateMargin(t)
		})

		// Test the complete function just in case
		require.Equal(t, int64(filledEntries), hashMapNumberOfEntries(m, &buffers, 3))
		validateMargin(t)
	}

	mapTypes := []ebpf.MapType{ebpf.Hash}
	mapSizes := []uint32{50, 10000}
	filledPercentages := []uint32{0, 10, 50, 100}
	keySizes := []uint32{1, 4, 16, 128, 256}
	valueSizes := []uint32{1, 4, 16, 256, 512}
	keyLimits := []uint32{0, 8192}
	valuesLimit := []uint32{0, 8192}

	for _, mapType := range mapTypes {
		t.Run(mapType.String(), func(t *testing.T) {
			for _, maxEntries := range mapSizes {
				for _, filledPercentage := range filledPercentages {
					for _, keySize := range keySizes {
						for _, valueSize := range valueSizes {
							for _, keyLimit := range keyLimits {
								for _, valueLimit := range valuesLimit {
									if (keyLimit == 0 && valueLimit != 0) || (keyLimit != 0 && valueLimit == 0) || (keyLimit > 0 && keyLimit < keySize) || (valueLimit > 0 && valueLimit < valueSize) {
										// Skip cases that don't make sense due to the limits
										continue
									}

									// Do not test cases where the number of keys would exceed the maximum number of keys possible with the given key size
									maxNumKeys := math.Pow(2, float64(keySize*8.0))
									if float64(maxEntries*filledPercentage/100) > maxNumKeys {
										continue
									}

									if keyLimit == 0 && valueLimit == 0 {
										// ensure that the batch size is not too small
										batchSize := min(keyLimit/keySize, valueLimit/valueSize)
										if batchSize < 8 {
											continue
										}
									}

									t.Run(fmt.Sprintf("%dMaxEntries/%dPercFilled/%dKeySize/%dValueSize/%dKeyLimit/%dValueLimit", maxEntries, filledPercentage, keySize, valueSize, keyLimit, valueLimit), func(t *testing.T) {
										testInner(t, mapType, maxEntries*filledPercentage/100, maxEntries, keySize, valueSize, keyLimit, valueLimit)
									})
								}
							}
						}
					}
				}
			}
		})
	}
}
