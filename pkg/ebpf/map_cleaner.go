// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"sync"
	"time"

	cebpf "github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// MapCleaner is responsible for periodically sweeping an eBPF map
// and deleting entries that satisfy a certain predicate function supplied by the user
type MapCleaner[K any, V any] struct {
	emap      *GenericMap[K, V]
	batchSize uint32

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

	m, err := Map[K, V](emap)
	if err != nil {
		return nil, err
	}

	return &MapCleaner[K, V]{
		emap:      m,
		batchSize: batchSize,
		done:      make(chan struct{}),
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
	if BatchAPISupported() {
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
	var key K
	var val V
	totalCount, deletedCount := 0, 0
	it := mc.emap.IterateWithBatchSize(int(mc.batchSize))

	for it.Next(&key, &val) {
		if !shouldClean(nowTS, key, val) {
			continue
		}

		keysToDelete = append(keysToDelete, key)
	}

	var deletionError error
	if len(keysToDelete) > 0 {
		deletedCount, deletionError = mc.emap.BatchDelete(keysToDelete)
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
	var key K
	var val V
	totalCount, deletedCount := 0, 0

	entries := mc.emap.Iterate()
	for entries.Next(&key, &val) {
		totalCount++
		if !shouldClean(nowTS, key, val) {
			continue
		}
		keysToDelete = append(keysToDelete, key)
	}

	for _, k := range keysToDelete {
		err := mc.emap.Delete(&k)
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
