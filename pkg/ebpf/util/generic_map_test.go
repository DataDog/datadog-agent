// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package util

import (
	"fmt"
	"testing"

	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ebpfkernel "github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestBatchAPISupported(t *testing.T) {
	// Batch API is supported on kernels >= 5.6, so make sure that in those cases
	// it returns true
	kernelVersion, err := ebpfkernel.NewKernelVersion()
	require.NoError(t, err)

	if kernelVersion.IsRH7Kernel() || kernelVersion.IsRH8Kernel() {
		// Some of those kernels have backported the batch API, I don't want
		// to include those specifics in unit tests that are only meant to ensure
		// that the checks are correct in at least the basic cases.
		t.Skip("Unknown support for batch API on RHEL kernels")
	}

	require.Equal(t, kernelVersion.Code >= ebpfkernel.Kernel5_6, BatchAPISupported())
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

	it := m.IterateWithBatchSize(1)
	require.NotNil(t, it)
	require.IsType(t, &genericMapItemIterator[uint32, uint32]{}, it)
	for it.Next(&k, &v) {
		numElements++
		foundElements[k] = true
	}
	require.NoError(t, it.Err())

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
	expectedNumbers := make([]uint32, numsToPut)
	for i := uint32(0); i < numsToPut; i++ {
		require.NoError(t, m.Put(&i, &i))
		expectedNumbers[i] = i
	}

	var k uint32
	var v uint32
	actualNumbers := make([]uint32, numsToPut)

	it := m.IterateWithBatchSize(10)
	require.NotNil(t, it)
	require.IsType(t, &genericMapBatchIterator[uint32, uint32]{}, it)
	for it.Next(&k, &v) {
		actualNumbers[k] = v
	}

	require.NoError(t, it.Err())
	require.Equal(t, expectedNumbers, actualNumbers)
}

func TestBatchIterArray(t *testing.T) {
	if !BatchAPISupported() {
		t.Skip("Batch API not supported")
	}

	m, err := NewGenericMap[uint32, uint32](&ebpf.MapSpec{
		Type:       ebpf.Array,
		MaxEntries: 100,
	})
	require.NoError(t, err)

	numsToPut := uint32(50)
	expectedNumbers := make([]uint32, m.Map().MaxEntries())
	for i := uint32(0); i < numsToPut; i++ {
		val := i + 200 // To distinguish from unset values
		require.NoError(t, m.Put(&i, &val))
		expectedNumbers[i] = val
	}

	var k uint32
	var v uint32
	numElements := uint32(0)
	actualNumbers := make([]uint32, m.Map().MaxEntries())

	it := m.IterateWithBatchSize(10)
	require.NotNil(t, it)
	require.IsType(t, &genericMapBatchIterator[uint32, uint32]{}, it)
	for it.Next(&k, &v) {
		actualNumbers[k] = v
		numElements++
	}
	require.NoError(t, it.Err())

	// Array maps will return all values on iterations, even if they are unset
	require.Equal(t, m.Map().MaxEntries(), numElements)
	require.Equal(t, expectedNumbers, actualNumbers)
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
	expectedNumbers := make([]uint32, numsToPut)
	for i := uint32(0); i < numsToPut; i++ {
		require.NoError(t, m.Put(&i, &i))
		expectedNumbers[i] = i
	}

	var k uint32
	var v uint32
	actualNumbers := make([]uint32, numsToPut)

	it := m.IterateWithBatchSize(10)
	require.NotNil(t, it)
	require.IsType(t, &genericMapBatchIterator[uint32, uint32]{}, it)
	for it.Next(&k, &v) {
		actualNumbers[k] = v
	}

	require.NoError(t, it.Err())
	require.Equal(t, expectedNumbers, actualNumbers)
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

	it := m.IterateWithBatchSize(10)
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

	// Again, just concerned with exiting the loop and not correctness, we don't even
	// check for no error on end of iteration.
	require.LessOrEqual(t, numElements, uint32(maxEntries))
}

func TestIteratePerCPUMaps(t *testing.T) {
	kernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)

	if kernelVersion < kernel.VersionCode(4, 6, 0) {
		t.Skip("Per CPU maps not supported on this kernel version")
	}

	m, err := NewGenericMap[uint32, []uint32](&ebpf.MapSpec{
		Type:       ebpf.PerCPUHash,
		MaxEntries: 10,
	})
	require.NoError(t, err)

	nbCpus, err := kernel.PossibleCPUs()
	require.NoError(t, err)

	numsToPut := []uint32{0, 100, 200, 300, 400, 500}
	for _, num := range numsToPut {
		entries := make([]uint32, nbCpus)
		for i := 0; i < nbCpus; i++ {
			entries[i] = num + uint32(i)
		}
		require.NoError(t, m.Put(&num, &entries))
	}

	var k uint32
	entries := make([]uint32, nbCpus)
	numElements := 0
	foundElements := make(map[uint32]bool)

	it := m.Iterate()
	require.NotNil(t, it)
	require.IsType(t, &genericMapItemIterator[uint32, []uint32]{}, it)
	for it.Next(&k, &entries) {
		numElements++
		foundElements[k] = true

		for i := 0; i < nbCpus; i++ {
			require.Equal(t, k+uint32(i), entries[i])
		}
	}
	require.NoError(t, it.Err())

	require.Equal(t, len(numsToPut), numElements)
	for _, num := range numsToPut {
		require.True(t, foundElements[num])
	}
}

type ValueStruct struct {
	A uint32
	B uint32
}

func TestIterateWithValueStructs(t *testing.T) {
	doTest := func(t *testing.T, batchSize int, oneItem bool) {
		singleItem := batchSize == 1
		if !singleItem && !BatchAPISupported() {
			t.Skip("Batch API not supported")
		}

		m, err := NewGenericMap[uint32, ValueStruct](&ebpf.MapSpec{
			Type:       ebpf.Hash,
			MaxEntries: 10,
		})
		require.NoError(t, err)

		var numsToPut []uint32
		if oneItem {
			numsToPut = []uint32{10}
		} else {
			numsToPut = []uint32{0, 100, 200, 300, 400, 500}
		}
		for _, num := range numsToPut {
			v := ValueStruct{A: num, B: num + 1}
			require.NoError(t, m.Put(&num, &v))
		}

		var k uint32
		var v ValueStruct
		numElements := 0
		foundElements := make(map[uint32]bool)

		it := m.IterateWithBatchSize(batchSize)
		require.NotNil(t, it)
		if singleItem {
			require.IsType(t, &genericMapItemIterator[uint32, ValueStruct]{}, it)
		} else {
			require.IsType(t, &genericMapBatchIterator[uint32, ValueStruct]{}, it)
		}

		for it.Next(&k, &v) {
			numElements++
			foundElements[k] = true

			require.Equal(t, k, v.A)
			require.Equal(t, k+1, v.B)
		}

		require.Equal(t, len(numsToPut), numElements)
		for _, num := range numsToPut {
			require.True(t, foundElements[num])
		}
	}

	t.Run("SingleItem", func(t *testing.T) {
		doTest(t, 1, false)
	})

	t.Run("Batch", func(t *testing.T) {
		doTest(t, 0, false)
	})

	t.Run("BatchWithOneItem", func(t *testing.T) {
		doTest(t, 0, true)
	})
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

	it := m.IterateWithBatchSize(batchSize)
	numElements := uint32(0)
	allocs := testing.AllocsPerRun(1, func() {
		for it.Next(&k, &v) {
			numElements++
		}
	})
	require.Equal(t, numsToPut, numElements)
	assert.EqualValues(t, allocs, 0)

	batchSize = 100
	numElements = uint32(0)
	it = m.IterateWithBatchSize(batchSize)
	allocs = testing.AllocsPerRun(1, func() {
		for it.Next(&k, &v) {
			numElements++
		}
	})
	require.Equal(t, numsToPut, numElements)
	assert.EqualValues(t, allocs, 0)
}

func BenchmarkIterate(b *testing.B) {
	setupAndBenchmark := func(b *testing.B, forceSingleItem bool, maxEntries int, numEntries int, batchSize int) {
		if !forceSingleItem && !BatchAPISupported() {
			b.Skip("Batch API not supported")
		}

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
			batchSize = 1
		} else {
			benchName = fmt.Sprintf("BenchmarkIterateBatch-%dentries-%dbatch", numEntries, batchSize)
		}

		b.Run(benchName, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				var k uint32
				var v uint32

				it := m.IterateWithBatchSize(batchSize)
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

func TestBatchDelete(t *testing.T) {
	if !BatchAPISupported() {
		t.Skip("Batch API not supported")
	}

	m, err := NewGenericMap[uint32, uint32](&ebpf.MapSpec{
		Type:       ebpf.Hash,
		MaxEntries: 100,
	})
	require.NoError(t, err)

	numsToPut := uint32(90)
	for i := uint32(0); i < numsToPut; i++ {
		require.NoError(t, m.Put(&i, &i))
	}

	numsToDelete := uint32(10)
	toDelete := make([]uint32, numsToDelete)
	for i := uint32(0); i < numsToDelete; i++ {
		toDelete[i] = i
	}
	deleted, err := m.BatchDelete(toDelete)
	require.NoError(t, err)
	require.Equal(t, numsToDelete, uint32(deleted))

	var k uint32
	var v uint32
	numElements := uint32(0)
	foundElements := make(map[uint32]bool)

	it := m.IterateWithBatchSize(1)
	require.NotNil(t, it)
	require.IsType(t, &genericMapItemIterator[uint32, uint32]{}, it)
	for it.Next(&k, &v) {
		numElements++
		foundElements[k] = true
	}

	require.Equal(t, numsToPut-numsToDelete, numElements)
	for i := uint32(0); i < numsToPut; i++ {
		require.Equal(t, foundElements[i], i >= numsToDelete)
	}
}

func TestBatchUpdate(t *testing.T) {
	if !BatchAPISupported() {
		t.Skip("Batch API not supported")
	}

	m, err := NewGenericMap[uint32, uint32](&ebpf.MapSpec{
		Type:       ebpf.Hash,
		MaxEntries: 100,
	})
	require.NoError(t, err)

	numsToCreate := uint32(90)
	toCreateKeys := make([]uint32, numsToCreate)
	toCreateValues := make([]uint32, numsToCreate)
	for i := uint32(0); i < numsToCreate; i++ {
		toCreateKeys[i] = i
		toCreateValues[i] = i
	}
	updated, err := m.BatchUpdate(toCreateKeys, toCreateValues, nil)
	require.NoError(t, err)
	require.Equal(t, numsToCreate, uint32(updated))

	var k uint32
	var v uint32
	numElements := uint32(0)
	foundElements := make(map[uint32]bool)

	it := m.IterateWithBatchSize(1)
	require.NotNil(t, it)
	require.IsType(t, &genericMapItemIterator[uint32, uint32]{}, it)
	for it.Next(&k, &v) {
		numElements++
		foundElements[k] = true
	}

	require.Equal(t, numsToCreate, numElements)
	for i := uint32(0); i < numsToCreate; i++ {
		require.True(t, foundElements[i])
	}
}
