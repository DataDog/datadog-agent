// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"sync"
	"time"
	"unsafe"

	cebpf "github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// MapCleaner is responsible for periodically sweeping an eBPF map
// and deleting entries that satisfy a certain predicate function supplied by the user
type MapCleaner[K any, V any] struct {
	emap        *cebpf.Map
	keyBatch    []K
	valuesBatch []V

	once sync.Once

	// termination
	stopOnce sync.Once
	done     chan struct{}
}

// NewMapCleaner instantiates a new MapCleaner
func NewMapCleaner[K any, V any](emap *cebpf.Map, defaultBatchSize uint32) (*MapCleaner[K, V], error) {
	batchSize := defaultBatchSize
	if defaultBatchSize > emap.MaxEntries() {
		batchSize = emap.MaxEntries()
	}
	if batchSize == 0 {
		batchSize = 1
	}

	return &MapCleaner[K, V]{
		emap:        emap,
		keyBatch:    make([]K, batchSize),
		valuesBatch: make([]V, batchSize),
		done:        make(chan struct{}),
	}, nil
}

// Clean eBPF map
// `interval` determines how often the eBPF map is scanned;
// `shouldClean` is a predicate method that determines whether a certain
// map entry should be deleted. the callback argument `nowTS` can be directly
// compared to timestamps generated using the `bpf_ktime_get_ns()` helper;
// `preClean` callback (optional, can pass nil) is invoked before the map is scanned; if it returns false,
// the map is not scanned; this can be used to synchronize with other maps, or preform preliminary checks.
// `postClean` callback (optional, can pass nil) is invoked after the map is scanned, to allow resource cleanup.
func (mc *MapCleaner[K, V]) Clean(interval time.Duration, preClean func() bool, postClean func(), shouldClean func(nowTS int64, k K, v V) bool) {
	if mc == nil {
		return
	}

	// Since kernel 5.6, the eBPF library supports batch operations on maps, which reduces the number of syscalls
	// required to clean the map. We use the new batch operations if the kernel version is >= 5.6, and fallback to
	// the old method otherwise. The new API is also more efficient because it minimizes the number of allocations.
	cleaner := mc.cleanWithoutBatches
	if version, err := kernel.HostVersion(); err == nil && version >= kernel.VersionCode(5, 6, 0) {
		cleaner = mc.cleanWithBatches
	}

	mc.once.Do(func() {
		ticker := time.NewTicker(interval)
		go func() {
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					now, err := NowNanoseconds()
					if err != nil {
						break
					}
					// Allowing to prepare for the cleanup.
					if preClean != nil && !preClean() {
						continue
					}
					cleaner(now, shouldClean)
					// Allowing cleanup after the cleanup.
					if postClean != nil {
						postClean()
					}
				case <-mc.done:
					return
				}
			}
		}()
	})
}

// Stop stops the map cleaner
func (mc *MapCleaner[K, V]) Stop() {
	if mc == nil {
		return
	}

	mc.stopOnce.Do(func() {
		// send to channel to synchronize with goroutine doing the map cleaning
		mc.done <- struct{}{}
		close(mc.done)
	})
}

func (mc *MapCleaner[K, V]) cleanWithBatches(nowTS int64, shouldClean func(nowTS int64, k K, v V) bool) {
	now := time.Now()

	var keysToDelete []K
	totalCount, deletedCount := 0, 0
	var cursor cebpf.BatchCursor
	var n int
	for {
		n, _ = mc.emap.BatchLookup(&cursor, mc.keyBatch, mc.valuesBatch, nil)
		if n == 0 {
			break
		}

		totalCount += n
		for i := 0; i < n; i++ {
			if !shouldClean(nowTS, mc.keyBatch[i], mc.valuesBatch[i]) {
				continue
			}
			keysToDelete = append(keysToDelete, mc.keyBatch[i])
		}

		// Just a safety check to avoid an infinite loop.
		if totalCount >= int(mc.emap.MaxEntries()) {
			break
		}
	}

	var deletionError error
	if len(keysToDelete) > 0 {
		deletedCount, deletionError = mc.emap.BatchDelete(keysToDelete, nil)
	}

	elapsed := time.Since(now)
	log.Debugf(
		"finished cleaning map=%s entries_checked=%d entries_deleted=%d deletion_error='%v' elapsed=%s",
		mc.emap,
		totalCount,
		deletedCount,
		deletionError,
		elapsed,
	)
}

func (mc *MapCleaner[K, V]) cleanWithoutBatches(nowTS int64, shouldClean func(nowTS int64, k K, v V) bool) {
	now := time.Now()

	var keysToDelete []K
	totalCount, deletedCount := 0, 0

	entries := mc.emap.Iterate()
	// we resort to unsafe.Pointers because by doing so the underlying eBPF
	// library avoids marshaling the key/value variables while traversing the map
	for entries.Next(unsafe.Pointer(&mc.keyBatch[0]), unsafe.Pointer(&mc.valuesBatch[0])) {
		totalCount++
		if !shouldClean(nowTS, mc.keyBatch[0], mc.valuesBatch[0]) {
			continue
		}
		keysToDelete = append(keysToDelete, mc.keyBatch[0])
	}

	for _, key := range keysToDelete {
		err := mc.emap.Delete(unsafe.Pointer(&key))
		if err == nil {
			deletedCount++
		}
	}

	elapsed := time.Since(now)
	log.Debugf(
		"finished cleaning map=%s entries_checked=%d entries_deleted=%d elapsed=%s",
		mc.emap,
		totalCount,
		deletedCount,
		elapsed,
	)
}
