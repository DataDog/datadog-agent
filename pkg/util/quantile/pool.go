// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package quantile

import (
	"sync"

	"github.com/DataDog/sketches-go/ddsketch/store"
)

const (
	defaultBinListSize      = 2 * defaultBinLimit
	defaultKeyListSize      = 256
	defaultOverflowListSize = 16
)

var (
	// TODO: multiple pools, one for each size class (like github.com/oxtoacart/bpool)
	binListPool = sync.Pool{
		New: func() interface{} {
			a := make([]bin, 0, defaultBinListSize)
			return &a
		},
	}

	keyListPool = sync.Pool{
		New: func() interface{} {
			a := make([]Key, 0, defaultKeyListSize)
			return &a
		},
	}

	overflowListPool = sync.Pool{
		New: func() interface{} {
			a := make([]bin, 0, defaultOverflowListSize)
			return &a
		},
	}
)

func getBinList() []bin {
	a := *(binListPool.Get().(*[]bin))
	return a[:0]
}

func putBinList(a []bin) {
	binListPool.Put(&a)
}

func getKeyList() []Key {
	a := *(keyListPool.Get().(*[]Key))
	return a[:0]
}

func putKeyList(a []Key) {
	keyListPool.Put(&a)
}

func getOverflowList() []bin {
	a := *(overflowListPool.Get().(*[]bin))
	return a[:0]
}

func putOverflowList(a []bin) {
	overflowListPool.Put(&a)
}

var floatKeyCountPool = sync.Pool{
	New: func() interface{} {
		a := make([]floatKeyCount, 0, defaultBinListSize)
		return &a
	},
}

func getFloatKeyCountList() []floatKeyCount {
	a := *(floatKeyCountPool.Get().(*[]floatKeyCount))
	return a[:0]
}

func putFloatKeyCountList(a []floatKeyCount) {
	if cap(a) >= defaultBinListSize {
		floatKeyCountPool.Put(&a)
	}
}

var keyCountPool = sync.Pool{
	New: func() interface{} {
		a := make([]KeyCount, 0, defaultBinListSize)
		return &a
	},
}

func getKeyCountList() []KeyCount {
	a := *(keyCountPool.Get().(*[]KeyCount))
	return a[:0]
}

func putKeyCountList(a []KeyCount) {
	if cap(a) >= defaultBinListSize {
		keyCountPool.Put(&a)
	}
}

var denseStorePool = sync.Pool{
	New: func() interface{} {
		return store.NewDenseStore()
	},
}

func getDenseStore() store.Store {
	return denseStorePool.Get().(store.Store)
}

func putDenseStore(s store.Store) {
	denseStorePool.Put(s)
}
