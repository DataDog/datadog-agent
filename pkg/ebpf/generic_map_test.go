// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/require"
)

func TestBatchAPISupported(t *testing.T) {
	// Batch API is supported on kernels >= 5.6, so make sure that in those cases
	// it returns true
	kernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)

	if kernelVersion <= kernel.VersionCode(5, 6, 0) {
		require.False(t, BatchAPISupported())
	} else {
		require.True(t, BatchAPISupported())
	}
}

func TestSingleItemIter(t *testing.T) {
	m, err := NewGenericMap[uint32, uint32](&ebpf.MapSpec{
		Type:       ebpf.Hash,
		MaxEntries: 10,
	})
	require.NoError(t, err)

	numsToPut := []uint32{1, 2, 3, 4, 5}
	for _, num := range numsToPut {
		require.NoError(t, m.Put(&num, &num))
	}

	var k uint32
	var v uint32
	numElements := 0
	foundElements := make(map[uint32]bool)

	it := m.IterateWithOptions(IteratorOptions{ForceSingleItem: true})
	require.NotNil(t, it)
	require.IsType(t, &genericMapItemIterator[uint32, uint32]{}, it)
	for it.Next(&k, &v) {
		numElements++
		foundElements[k] = true
	}

	require.Equal(t, len(numsToPut), numElements)
	for _, num := range numsToPut {
		require.True(t, foundElements[num])
	}
}

func TestBatchIter(t *testing.T) {
	if !BatchAPISupported() {
		t.Skip("Batch API not supported")
	}

	m, err := NewGenericMap[uint32, uint32](&ebpf.MapSpec{
		Type:       ebpf.Hash,
		MaxEntries: 100,
	})
	require.NoError(t, err)

	numsToPut := uint32(50)
	for i := uint32(0); i < numsToPut; i++ {
		require.NoError(t, m.Put(&i, &i))
	}

	var k uint32
	var v uint32
	numElements := uint32(0)
	foundElements := make(map[uint32]bool)

	it := m.IterateWithOptions(IteratorOptions{BatchSize: 10})
	require.NotNil(t, it)
	require.IsType(t, &genericMapBatchIterator[uint32, uint32]{}, it)
	for it.Next(&k, &v) {
		numElements++
		foundElements[k] = true
	}

	require.Equal(t, numsToPut, numElements)
	for i := uint32(0); i < numsToPut; i++ {
		require.True(t, foundElements[i])
	}
}

func TestBatchIterLessItemsThanBatchSize(t *testing.T) {
	if !BatchAPISupported() {
		t.Skip("Batch API not supported")
	}

	m, err := NewGenericMap[uint32, uint32](&ebpf.MapSpec{
		Type:       ebpf.Hash,
		MaxEntries: 100,
	})
	require.NoError(t, err)

	numsToPut := uint32(5)
	for i := uint32(0); i < numsToPut; i++ {
		require.NoError(t, m.Put(&i, &i))
	}

	var k uint32
	var v uint32
	numElements := uint32(0)
	foundElements := make(map[uint32]bool)

	it := m.IterateWithOptions(IteratorOptions{BatchSize: 10})
	require.NotNil(t, it)
	require.IsType(t, &genericMapBatchIterator[uint32, uint32]{}, it)
	for it.Next(&k, &v) {
		numElements++
		foundElements[k] = true
	}

	require.Equal(t, numsToPut, numElements)
	for i := uint32(0); i < numsToPut; i++ {
		require.True(t, foundElements[i])
	}
}

func TestBatchIterWhileUpdated(t *testing.T) {
	if !BatchAPISupported() {
		t.Skip("Batch API not supported")
	}

	maxEntries := 50
	m, err := NewGenericMap[uint32, uint32](&ebpf.MapSpec{
		Type:       ebpf.Hash,
		MaxEntries: uint32(maxEntries),
	})
	require.NoError(t, err)

	numsToPut := uint32(50)
	for i := uint32(0); i < numsToPut; i++ {
		require.NoError(t, m.Put(&i, &i))
	}

	var k uint32
	var v uint32
	numElements := uint32(0)
	foundElements := make(map[uint32]bool)
	updateEachElements := 25
	updatesDone := 0

	it := m.IterateWithOptions(IteratorOptions{BatchSize: 10})
	require.NotNil(t, it)
	require.IsType(t, &genericMapBatchIterator[uint32, uint32]{}, it)
	for it.Next(&k, &v) {
		numElements++
		foundElements[k] = true

		// Not recommended! But helps us simulate the case where the map is updated
		// as we iterate over it. We are not concerned with correctness here but we
		// want to make sure that the iterator doesn't crash or run into an infinite
		// loop
		if numElements%uint32(updateEachElements) == 0 {
			for i := uint32(0); i < numsToPut; i++ {
				oldKey := i + uint32(updatesDone)*10
				newKey := i + uint32(updatesDone+1)*10
				require.NoError(t, m.Delete(&oldKey))
				require.NoError(t, m.Put(&newKey, &newKey))
			}

			updatesDone++
		}

		require.LessOrEqual(t, numElements, uint32(maxEntries))
	}

	// Again, just concerned with exiting the loop and not correctness
	require.LessOrEqual(t, numElements, uint32(maxEntries))
}

func TestBatchIterAllocsPerRun(t *testing.T) {
	if !BatchAPISupported() {
		t.Skip("Batch API not supported")
	}

	m, err := NewGenericMap[uint32, uint32](&ebpf.MapSpec{
		Type:       ebpf.Hash,
		MaxEntries: 10000,
	})
	require.NoError(t, err)

	numsToPut := uint32(9000)
	for i := uint32(0); i < numsToPut; i++ {
		require.NoError(t, m.Put(&i, &i))
	}

	var k uint32
	var v uint32
	batchSize := 10

	allocsSmallBatch := testing.AllocsPerRun(100, func() {
		numElements := uint32(0)
		it := m.IterateWithOptions(IteratorOptions{BatchSize: batchSize})
		for it.Next(&k, &v) {
			numElements++
		}
		require.Equal(t, numsToPut, numElements)
	})

	batchSize = 100

	allocsLargerBatch := testing.AllocsPerRun(100, func() {
		numElements := uint32(0)
		it := m.IterateWithOptions(IteratorOptions{BatchSize: batchSize})
		for it.Next(&k, &v) {
			numElements++
		}
		require.Equal(t, numsToPut, numElements)
	})

	require.LessOrEqual(t, allocsSmallBatch, 8.0)
	require.LessOrEqual(t, allocsLargerBatch, 8.0)
	require.Equal(t, allocsLargerBatch, allocsSmallBatch) // We don't want allocations to depend on batch size
}

func BenchmarkIterate(b *testing.B) {
	setupAndBenchmark := func(b *testing.B, forceSingleItem bool, maxEntries int, numEntries int, batchSize int) {
		m, err := NewGenericMap[uint32, uint32](&ebpf.MapSpec{
			Type:       ebpf.Hash,
			MaxEntries: uint32(maxEntries),
		})
		require.NoError(b, err)

		for i := uint32(0); i < uint32(numEntries); i++ {
			require.NoError(b, m.Put(&i, &i))
		}

		var benchName string
		if forceSingleItem {
			benchName = fmt.Sprintf("BenchmarkIterateSingleItem-%dentries-%dbatch", numEntries, batchSize)
		} else {
			benchName = fmt.Sprintf("BenchmarkIterateBatch-%dentries-%dbatch", numEntries, batchSize)
		}

		b.Run(benchName, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				var k uint32
				var v uint32

				it := m.IterateWithOptions(IteratorOptions{BatchSize: batchSize, ForceSingleItem: forceSingleItem})
				for it.Next(&k, &v) {
				}
			}
		})
	}

	batchSizes := []int{5, 10, 20, 50, 100}
	for _, batchSize := range batchSizes {
		for _, numEntries := range []int{100, 1000, 10000} {
			if BatchAPISupported() {
				setupAndBenchmark(b, false, numEntries, numEntries, batchSize)
			}

			setupAndBenchmark(b, true, numEntries, numEntries, batchSize)
		}
	}
}
