// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"errors"
	"sync"
	"time"

	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const ebpfMapsCleanerModule = "ebpf__maps__cleaner"

var defaultBuckets = []float64{10, 25, 50, 75, 100, 250, 500, 1000, 10000}
var mapCleanerTelemetry = struct {
	examined telemetry.Counter
	deleted  telemetry.Counter
	aborts   telemetry.Counter
	elapsed  telemetry.Histogram
}{
	telemetry.NewCounter(ebpfMapsCleanerModule, "examined", []string{"map_name", "module", "api"}, "Counter measuring how many entries are examined"),
	telemetry.NewCounter(ebpfMapsCleanerModule, "deleted", []string{"map_name", "module", "api"}, "Counter measuring how many entries are deleted"),
	telemetry.NewCounter(ebpfMapsCleanerModule, "aborts", []string{"map_name", "module", "api"}, "Counter measuring how many iteration aborts occur"),
	telemetry.NewHistogram(ebpfMapsCleanerModule, "elapsed", []string{"map_name", "module", "api"}, "Histogram of elapsed time for each Clean call", defaultBuckets),
}

// MapCleaner is responsible for periodically sweeping an eBPF map
// and deleting entries that satisfy a certain predicate function supplied by the user
type MapCleaner[K any, V any] struct {
	emap      *maps.GenericMap[K, V]
	batchSize uint32

	// useBatchAPI determines whether the cleaner will use the batch API for iteration and deletion.
	useBatchAPI bool

	once sync.Once

	// termination
	stopOnce sync.Once
	done     chan struct{}

	examined      telemetry.SimpleCounter
	singleDeleted telemetry.SimpleCounter
	batchDeleted  telemetry.SimpleCounter
	aborts        telemetry.SimpleCounter
	elapsed       telemetry.SimpleHistogram

	cleanerFunc func(nowTS int64, shouldClean func(nowTS int64, k K, v V) bool)
}

// NewMapCleaner instantiates a new MapCleaner. defaultBatchSize controls the
// batch size for iteration of the map. If it is set to 1, the batch API will
// not be used for iteration nor for deletion.
func NewMapCleaner[K any, V any](emap *ebpf.Map, defaultBatchSize uint32, name, module string) (*MapCleaner[K, V], error) {
	batchSize := defaultBatchSize
	if defaultBatchSize > emap.MaxEntries() {
		batchSize = emap.MaxEntries()
	}
	if batchSize == 0 {
		batchSize = 1
	}

	m, err := maps.Map[K, V](emap)
	if err != nil {
		return nil, err
	}

	useBatchAPI := batchSize > 1 && m.CanUseBatchAPI()

	singleTags := map[string]string{"map_name": name, "module": module, "api": "single"}
	batchTags := map[string]string{"map_name": name, "module": module, "api": "batch"}
	tags := singleTags
	if useBatchAPI {
		tags = batchTags
	}

	cleaner := &MapCleaner[K, V]{
		emap:          m,
		batchSize:     batchSize,
		done:          make(chan struct{}),
		examined:      mapCleanerTelemetry.examined.WithTags(tags),
		singleDeleted: mapCleanerTelemetry.deleted.WithTags(singleTags),
		batchDeleted:  mapCleanerTelemetry.deleted.WithTags(batchTags),
		aborts:        mapCleanerTelemetry.aborts.WithTags(tags),
		elapsed:       mapCleanerTelemetry.elapsed.WithTags(tags),
		useBatchAPI:   useBatchAPI,
	}

	// Since kernel 5.6, the eBPF library supports batch operations on maps, which reduces the number of syscalls
	// required to clean the map. We use the new batch operations if they are supported (we check with a feature test instead
	// of a version comparison because some distros have backported this API), and fallback to
	// the old method otherwise. The new API is also more efficient because it minimizes the number of allocations.
	cleaner.cleanerFunc = cleaner.cleanWithoutBatches
	if useBatchAPI {
		cleaner.cleanerFunc = cleaner.cleanWithBatches
	}

	return cleaner, nil
}

// Start eBPF map periodically.
// `interval` determines how often the eBPF map is scanned;
// `shouldClean` is a predicate method that determines whether a certain
// map entry should be deleted. the callback argument `nowTS` can be directly
// compared to timestamps generated using the `bpf_ktime_get_ns()` helper;
// `preClean` callback (optional, can pass nil) is invoked before the map is scanned; if it returns false,
// the map is not scanned; this can be used to synchronize with other maps, or preform preliminary checks.
// `postClean` callback (optional, can pass nil) is invoked after the map is scanned, to allow resource cleanup.
func (mc *MapCleaner[K, V]) Start(interval time.Duration, preClean func() bool, postClean func(), shouldClean func(nowTS int64, k K, v V) bool) {
	if mc == nil {
		return
	}

	mc.once.Do(func() {
		ticker := time.NewTicker(interval)
		go func() {
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					mc.Clean(preClean, postClean, shouldClean)
				case <-mc.done:
					return
				}
			}
		}()
	})
}

// Clean eBPF map on demand.
// `shouldClean` is a predicate method that determines whether a certain
// map entry should be deleted. the callback argument `nowTS` can be directly
// compared to timestamps generated using the `bpf_ktime_get_ns()` helper;
// `preClean` callback (optional, can pass nil) is invoked before the map is scanned; if it returns false,
// the map is not scanned; this can be used to synchronize with other maps, or preform preliminary checks.
// `postClean` callback (optional, can pass nil) is invoked after the map is scanned, to allow resource cleanup.
func (mc *MapCleaner[K, V]) Clean(preClean func() bool, postClean func(), shouldClean func(nowTS int64, k K, v V) bool) {
	now, err := NowNanoseconds()
	if err != nil {
		return
	}
	// Allowing to prepare for the cleanup.
	if preClean != nil && !preClean() {
		return
	}
	mc.cleanerFunc(now, shouldClean)
	// Allowing cleanup after the cleanup.
	if postClean != nil {
		postClean()
	}
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
	start := time.Now()

	var keysToDelete []K
	var key K
	var val V
	totalCount, batchDeletedCount, singleDeletedCount := 0, 0, 0
	it := mc.emap.IterateWithBatchSize(int(mc.batchSize))

	for it.Next(&key, &val) {
		totalCount++
		if !shouldClean(nowTS, key, val) {
			continue
		}

		keysToDelete = append(keysToDelete, key)
	}

	if err := it.Err(); err != nil {
		if errors.Is(err, ebpf.ErrIterationAborted) {
			mc.aborts.Inc()
		} else {
			log.Errorf("error iterating map=%s: %s", mc.emap, err)
		}
	}

	if len(keysToDelete) > 0 {
		var deletionError error
		batchDeletedCount, deletionError = mc.emap.BatchDelete(keysToDelete)
		// We might have a partial deletion (as a key might be missing due to other cleaning mechanism), so we want
		// to have a best-effort method to delete all keys. We cannot know which keys were deleted, so we have to try
		// and delete all of them one by one.
		if errors.Is(deletionError, ebpf.ErrKeyNotExist) {
			for _, k := range keysToDelete {
				if err := mc.emap.Delete(&k); err == nil {
					singleDeletedCount++
				}
			}
		}
	}

	mc.examined.Add(float64(totalCount))
	mc.batchDeleted.Add(float64(batchDeletedCount))
	mc.singleDeleted.Add(float64(singleDeletedCount))
	mc.elapsed.Observe(float64(time.Since(start).Microseconds()))
}

func (mc *MapCleaner[K, V]) cleanWithoutBatches(nowTS int64, shouldClean func(nowTS int64, k K, v V) bool) {
	start := time.Now()

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

	if err := entries.Err(); err != nil {
		if errors.Is(err, ebpf.ErrIterationAborted) {
			mc.aborts.Inc()
		} else {
			log.Errorf("error iterating map=%s: %s", mc.emap, err)
		}
	}

	for _, k := range keysToDelete {
		err := mc.emap.Delete(&k)
		if err == nil {
			deletedCount++
		}
	}

	mc.examined.Add(float64(totalCount))
	mc.singleDeleted.Add(float64(deletedCount))
	mc.elapsed.Observe(float64(time.Since(start).Microseconds()))
}
