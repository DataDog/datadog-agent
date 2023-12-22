// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpf

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
)

var (
	batchAPISupportedOnce sync.Once
	batchAPISupported     bool
)

// BatchAPISupported returns true if the kernel supports the batch API for maps
func BatchAPISupported() bool {
	batchAPISupportedOnce.Do(func() {
		// Do feature detection directly instead of based on kernel versions for more accuracy
		m, err := ebpf.NewMap(&ebpf.MapSpec{
			Type:       ebpf.Hash,
			KeySize:    4,
			ValueSize:  4,
			MaxEntries: 10,
		})
		if err != nil {
			log.Warnf("Failed to create map for batch API test: %v, will mark batch API as unsupported", err)
			batchAPISupported = false
			return
		}

		keys := make([]uint32, 1)
		values := make([]uint32, 1)

		// Do a batch update, check the result.
		// We do an update instead of a lookup because it's more reliable for detection
		_, err = m.BatchUpdate(keys, values, nil)
		batchAPISupported = err == nil
	})
	return batchAPISupported
}

// GenericMap is a wrapper around ebpf.Map that allows to use generic types.
// Also includes support for batch iterations
type GenericMap[K interface{}, V interface{}] struct {
	m *ebpf.Map
}

// NewGenericMap creates a new GenericMap with the given spec. Key and Value sizes are automatically
// inferred from the types of K and V.
func NewGenericMap[K interface{}, V interface{}](spec *ebpf.MapSpec) (*GenericMap[K, V], error) {
	// Automatic inference of sizes. We assume that K/V are simple types that
	// can be instantiated with no arguments
	var kval K
	var vval V
	spec.KeySize = uint32(reflect.TypeOf(kval).Size())
	spec.ValueSize = uint32(reflect.TypeOf(vval).Size())

	m, err := ebpf.NewMap(spec)
	if err != nil {
		return nil, err
	}

	return &GenericMap[K, V]{
		m: m,
	}, nil
}

// Map creates a new GenericMap from an existing ebpf.Map
func Map[K interface{}, V interface{}](m *ebpf.Map) *GenericMap[K, V] {
	return &GenericMap[K, V]{
		m: m,
	}
}

// GetMap gets the generic map with the given name from the manager
func GetMap[K interface{}, V interface{}](mgr *manager.Manager, name string) (*GenericMap[K, V], error) {
	m, _, err := mgr.GetMap(name)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, fmt.Errorf("not found")
	}
	return Map[K, V](m), nil
}

// Map returns the underlying ebpf.Map
func (g *GenericMap[K, V]) Map() *ebpf.Map {
	return g.m
}

// IteratorOptions are options for the Iterate method
type IteratorOptions struct {
	BatchSize       int  // Number of items to fetch per batch. If 0, use default value (100)
	ForceSingleItem bool // Force the use of the single item iterator even if the batch API is supported
}

// Put inserts a new key/value pair in the map. If the key already exists, the value is updated
func (g *GenericMap[K, V]) Put(key *K, value *V) error {
	return g.Update(key, value, ebpf.UpdateAny)
}

// Update updates the value of an existing key in the map. If the key doesn't exist, it returns an error
func (g *GenericMap[K, V]) Update(key *K, value *V, flags ebpf.MapUpdateFlags) error {
	return g.m.Update(unsafe.Pointer(key), unsafe.Pointer(value), flags)
}

// Lookup looks up a key in the map and returns the value. If the key doesn't exist, it returns an error
func (g *GenericMap[K, V]) Lookup(key *K, valueOut *V) error {
	return g.m.Lookup(unsafe.Pointer(key), unsafe.Pointer(valueOut))
}

// Delete deletes a key from the map. If the key doesn't exist, it returns an error
func (g *GenericMap[K, V]) Delete(key *K) error {
	return g.m.Delete(unsafe.Pointer(key))
}

// GenericMapIterator is an interface for iterating over a GenericMap
type GenericMapIterator[K interface{}, V interface{}] interface {
	// Next fills K and V with the next key/value pair in the map. It returns false if there are no more elements
	Next(key *K, value *V) bool

	// Err returns the last error that happened during iteration.
	Err() error
}

// TODO: This is copied from pkg/collector/corechecks/ebpf/probe/probe.go temporarily
// I feel like this should be in a generic package (ebpf-manager maybe?) but I'm not sure,
// so I'm leaving it here for now until PR review
func isPerCPU(typ ebpf.MapType) bool {
	switch typ {
	case ebpf.PerCPUHash, ebpf.PerCPUArray, ebpf.LRUCPUHash:
		return true
	}
	return false
}

// Iterate returns an iterator for the map, which transparenlty chooses between batch and single item
// iterations.
func (g *GenericMap[K, V]) Iterate(itops IteratorOptions) GenericMapIterator[K, V] {
	if itops.BatchSize == 0 {
		itops.BatchSize = 100 // Default value for batch sizes. Possibly needs more testing to find an optimal default
	}
	if itops.BatchSize > int(g.m.MaxEntries()) {
		itops.BatchSize = int(g.m.MaxEntries())
	}

	if BatchAPISupported() && !isPerCPU(g.m.Type()) && !itops.ForceSingleItem {
		it := &genericMapBatchIterator[K, V]{
			m:         g.m,
			batchSize: itops.BatchSize,
			keys:      make([]K, itops.BatchSize),
			values:    make([]V, itops.BatchSize),
		}

		// Do an initial copy of the keys/values slices to avoid allocations
		it.keysCopy = it.keys
		it.valuesCopy = it.values

		return it
	}

	return &genericMapItemIterator[K, V]{
		it: g.m.Iterate(),
	}
}

type genericMapItemIterator[K interface{}, V interface{}] struct {
	it *ebpf.MapIterator
}

func (g *genericMapItemIterator[K, V]) Next(key *K, value *V) bool {
	return g.it.Next(unsafe.Pointer(key), unsafe.Pointer(value))
}

func (g *genericMapItemIterator[K, V]) Err() error {
	return g.it.Err()
}

type genericMapBatchIterator[K interface{}, V interface{}] struct {
	m                *ebpf.Map
	batchSize        int
	cursor           ebpf.BatchCursor
	keys             []K
	values           []V
	keysCopy         any // A pointer to keys of type "any", used to avoid allocations when calling BatchLookup
	valuesCopy       any
	currentBatchSize int
	inBatchIndex     int
	err              error
	totalCount       int
	lastBatch        bool
}

func (g *genericMapBatchIterator[K, V]) Next(key *K, value *V) bool {
	// Safety check to avoid an infinite loop
	if g.totalCount >= int(g.m.MaxEntries()) {
		return false
	}

	// We have finished all the values in the current batch (or there wasn't any batch
	// to begin with with g.currentBatchSize == 0), so we need to fetch the next batch
	if g.inBatchIndex >= g.currentBatchSize {
		if g.lastBatch {
			return false
		}

		// Important! If we pass here g.keys/g.values, Go will create a copy of the slice
		// and will generate extra allocations. I am not entirely sure why it is doing that.
		g.currentBatchSize, g.err = g.m.BatchLookup(&g.cursor, g.keysCopy, g.valuesCopy, nil)
		g.inBatchIndex = 0
		if g.currentBatchSize == 0 {
			return false
		} else if g.err != nil && errors.Is(g.err, ebpf.ErrKeyNotExist) {
			// The lookup API returns ErrKeyNotExist when this is the last batch,
			// even when partial results are returned. We need to mark this so that
			// we don't try to fetch another batch when this one is finished
			g.lastBatch = true
		} else if g.err != nil {
			return false
		}
	}

	// At this point we know for sure that keys/values are populated with values
	// from a previous call to BatchLookup.
	*key = g.keys[g.inBatchIndex]
	*value = g.values[g.inBatchIndex]
	g.inBatchIndex++
	g.totalCount++

	return true
}

func (g *genericMapBatchIterator[K, V]) Err() error {
	return g.err
}
