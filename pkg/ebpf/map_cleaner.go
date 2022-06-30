// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package ebpf

import (
	"fmt"
	"reflect"
	"sync"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	cebpf "github.com/cilium/ebpf"
)

// MapCleaner is responsible for periodically sweeping an eBPF map
// and deleting entries that satisfy a certain predicate function supplied by the user
type MapCleaner struct {
	emap *cebpf.Map
	key  interface{}
	val  interface{}
	once sync.Once

	// we resort to unsafe.Pointers because by doing so the underlying eBPF
	// library avoids marshaling the key/value variables while traversing the
	// map, which not only has an overhead but also requires all fields from the
	// datatype to be exported
	keyPtr unsafe.Pointer
	valPtr unsafe.Pointer

	// termination
	stopOnce sync.Once
	done     chan struct{}
}

// NewMapCleaner instantiates a new MapCleaner
func NewMapCleaner(emap *cebpf.Map, key, val interface{}) (*MapCleaner, error) {
	// we force types to be of pointer kind because of the reasons mentioned above
	if reflect.ValueOf(key).Kind() != reflect.Ptr {
		return nil, fmt.Errorf("%T is not a pointer kind", key)
	}
	if reflect.ValueOf(val).Kind() != reflect.Ptr {
		return nil, fmt.Errorf("%T is not a pointer kind", val)
	}

	return &MapCleaner{
		emap:   emap,
		key:    key,
		val:    val,
		keyPtr: unsafe.Pointer(reflect.ValueOf(key).Elem().Addr().Pointer()),
		valPtr: unsafe.Pointer(reflect.ValueOf(val).Elem().Addr().Pointer()),
		done:   make(chan struct{}),
	}, nil
}

// Clean eBPF map
// `interval` determines how often the eBPF map is scanned;
// `shouldClean` is a predicate method that determines whether or not a certain
// map entry should be deleted. the callback argument `nowTS` can be directly
// compared to timestamps generated using the `bpf_ktime_get_ns()` helper;
func (mc *MapCleaner) Clean(interval time.Duration, shouldClean func(nowTS int64, k, v interface{}) bool) {
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
					// TODO: use pkg/security/probe/time_resolver.go to account for
					// clock drifts when system was suspended
					now, err := NowNanoseconds()
					if err != nil {
						break
					}
					mc.clean(now, shouldClean)
				case <-mc.done:
					return
				}
			}

		}()
	})
}

func (mc *MapCleaner) Stop() {
	if mc == nil {
		return
	}

	mc.stopOnce.Do(func() {
		mc.done <- struct{}{}
		close(mc.done)
	})
}

func (mc *MapCleaner) clean(nowTS int64, shouldClean func(nowTS int64, k, v interface{}) bool) error {
	totalCount, deletedCount := 0, 0
	entries := mc.emap.Iterate()
	now := time.Now()

	for entries.Next(mc.keyPtr, mc.valPtr) {
		totalCount++
		if shouldClean(nowTS, mc.key, mc.val) {
			err := mc.emap.Delete(mc.keyPtr)
			if err == nil {
				deletedCount++
			}
		}
	}

	if err := entries.Err(); err != nil {
		return err
	}

	elapsed := time.Now().Sub(now)
	log.Debugf(
		"finished cleaning map=%s entries_checked=%d entries_deleted=%d, elapsed=%s",
		mc.emap,
		totalCount,
		deletedCount,
		elapsed,
	)
	return nil
}
