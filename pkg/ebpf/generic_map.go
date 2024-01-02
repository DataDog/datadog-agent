// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

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

const defaultBatchSize = 100

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
// Important: if the map is a per-cpu map, V must be a slice type
func NewGenericMap[K interface{}, V interface{}](spec *ebpf.MapSpec) (*GenericMap[K, V], error) {
	// Automatic inference of sizes. We assume that K/V are simple types that
	// can be instantiated with no arguments
	var kval K
	var vval V

	err := validateValueTypeForMapType[V](spec.Type)
	if err != nil {
		return nil, err
	}

	spec.KeySize = uint32(unsafe.Sizeof(kval))

	if isPerCPU(spec.Type) {
		spec.ValueSize = uint32(reflect.TypeOf(vval).Elem().Size())
	} else {
		spec.ValueSize = uint32(unsafe.Sizeof(vval))
	}

	m, err := ebpf.NewMap(spec)
	if err != nil {
		return nil, err
	}

	return &GenericMap[K, V]{
		m: m,
	}, nil
}

func validateValueTypeForMapType[V interface{}](t ebpf.MapType) error {
	var vval V
	if isPerCPU(t) && reflect.TypeOf(vval).Kind() != reflect.Slice {
		return errors.New("per-cpu maps require a slice type for the value, instead got %T", vval)
	}
	return nil
}

// Map creates a new GenericMap from an existing ebpf.Map
func Map[K interface{}, V interface{}](m *ebpf.Map) (*GenericMap[K, V], error) {
	if err := validateValueTypeForMapType[V](m.Type()); err != nil {
		return nil, err
	}

	return &GenericMap[K, V]{
		m: m,
	}, nil
}

// GetMap gets the generic map with the given name from the manager
func GetMap[K interface{}, V interface{}](mgr *manager.Manager, name string) (*GenericMap[K, V], error) {
	m, _, err := mgr.GetMap(name)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, fmt.Errorf("map %q not found", name)
	}
	gm, err := Map[K, V](m)
	if err != nil {
		return nil, err
	}
	return gm, nil
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
	if g.isPerCPU() {
		return g.m.Put(unsafe.Pointer(key), *value)
	}

	return g.m.Put(unsafe.Pointer(key), unsafe.Pointer(value))
}

// Update updates the value of an existing key in the map.
func (g *GenericMap[K, V]) Update(key *K, value *V, flags ebpf.MapUpdateFlags) error {
	return g.m.Update(unsafe.Pointer(key), unsafe.Pointer(value), flags)
}

// Lookup looks up a key in the map and returns the value. If the key doesn't exist, it returns ErrKeyNotExist
func (g *GenericMap[K, V]) Lookup(key *K, valueOut *V) error {
	if g.isPerCPU() {
		return g.m.Lookup(unsafe.Pointer(key), *valueOut)
	}

	return g.m.Lookup(unsafe.Pointer(key), unsafe.Pointer(valueOut))
}

// Delete deletes a key from the map. If the key doesn't exist, it returns ErrKeyNotExist
func (g *GenericMap[K, V]) Delete(key *K) error {
	return g.m.Delete(unsafe.Pointer(key))
}

// BatchDelete deletes a batch of keys from the map
func (g *GenericMap[K, V]) BatchDelete(keys []K) (int, error) {
	return g.m.BatchDelete(keys, nil)
}

// GenericMapIterator is an interface for iterating over a GenericMap
type GenericMapIterator[K interface{}, V interface{}] interface {
	// Next fills K and V with the next key/value pair in the map. It returns false if there are no more elements
	Next(key *K, value *V) bool

	// Err returns the last error that happened during iteration.
	Err() error
}

func isPerCPU(t ebpf.MapType) bool {
	switch t {
	case ebpf.PerCPUHash, ebpf.PerCPUArray, ebpf.LRUCPUHash:
		return true
	}
	return false
}

func (g *GenericMap[K, V]) isPerCPU() bool {
	return isPerCPU(g.m.Type())
}

// Iterate returns an iterator for the map, which transparently chooses between batch and single item
func (g *GenericMap[K, V]) Iterate() GenericMapIterator[K, V] {
	return g.IterateWithOptions(IteratorOptions{BatchSize: defaultBatchSize})
}

func (g *GenericMap[K, V]) valueTypeCanUseUnsafePointer() bool {
	// Simple test for now, but we probably will need to add more cases,
	// as I am not 100% sure of the behavior of structs with maps
	return !g.isPerCPU() // PerCPU maps use slices, so we need to pass them directly
}

// IterateWithOptions returns an iterator for the map, which transparently chooses between batch and single item
// iterations. This version allows choosing options
func (g *GenericMap[K, V]) IterateWithOptions(itops IteratorOptions) GenericMapIterator[K, V] {
	if itops.BatchSize == 0 {
		itops.BatchSize = defaultBatchSize // Default value for batch sizes. Possibly needs more testing to find an optimal default
	}
	if itops.BatchSize > int(g.m.MaxEntries()) {
		itops.BatchSize = int(g.m.MaxEntries())
	}

	if BatchAPISupported() && !g.isPerCPU() && !itops.ForceSingleItem {
		it := &genericMapBatchIterator[K, V]{
			m:                            g.m,
			batchSize:                    itops.BatchSize,
			keys:                         make([]K, itops.BatchSize),
			values:                       make([]V, itops.BatchSize),
			valueTypeCanUseUnsafePointer: g.valueTypeCanUseUnsafePointer(),
		}

		// Do an initial copy of the keys/values slices to avoid allocations
		it.keysCopy = it.keys
		it.valuesCopy = it.values

		return it
	}

	return &genericMapItemIterator[K, V]{
		it:                           g.m.Iterate(),
		valueTypeCanUseUnsafePointer: g.valueTypeCanUseUnsafePointer(),
	}
}

type genericMapItemIterator[K interface{}, V interface{}] struct {
	it                           *ebpf.MapIterator
	valueTypeCanUseUnsafePointer bool
}

func (g *genericMapItemIterator[K, V]) Next(key *K, value *V) bool {
	// we resort to unsafe.Pointers because by doing so the underlying eBPF
	// library avoids marshaling the key/value variables while traversing the map
	// However, in some cases (slices, structs) we need to pass the variable directly
	// so that the library detects the type correctly
	if g.valueTypeCanUseUnsafePointer {
		return g.it.Next(unsafe.Pointer(key), unsafe.Pointer(value))
	}

	return g.it.Next(unsafe.Pointer(key), value)
}

func (g *genericMapItemIterator[K, V]) Err() error {
	return g.it.Err()
}

type genericMapBatchIterator[K interface{}, V interface{}] struct {
	m                            *ebpf.Map
	batchSize                    int
	cursor                       ebpf.BatchCursor
	keys                         []K
	values                       []V
	keysCopy                     any // A pointer to keys of type "any", used to avoid allocations when calling BatchLookup
	valuesCopy                   any
	currentBatchSize             int
	inBatchIndex                 int
	err                          error
	totalCount                   int
	lastBatch                    bool
	valueTypeCanUseUnsafePointer bool
}

func (g *genericMapBatchIterator[K, V]) Next(key *K, value *V) bool {
	// Safety check to avoid an infinite loop
	if g.totalCount >= int(g.m.MaxEntries()) {
		return false
	}

	// We have finished all the values in the current batch (or there wasn't any batch
	// to begin with, with g.currentBatchSize == 0), so we need to fetch the next batch
	if g.inBatchIndex >= g.currentBatchSize {
		if g.lastBatch {
			return false
		}

		// Important! If we pass here g.keys/g.values, Go will create a copy of the slice instance
		// and will generate extra allocations. I am not entirely sure why it is doing that.
		g.currentBatchSize, g.err = g.m.BatchLookup(&g.cursor, g.keysCopy, g.valuesCopy, nil)
		g.inBatchIndex = 0
		if g.err != nil && errors.Is(g.err, ebpf.ErrKeyNotExist) {
			// The lookup API returns ErrKeyNotExist when this is the last batch,
			// even when partial results are returned. We need to mark this so that
			// we don't try to fetch another batch when this one is finished
			g.lastBatch = true

			// Also fix the error, because in some instances BatchLookup sets ErrKeyNotExist
			// as the error, which is just an indicator that there are no more batches, but it's not
			// an actual error.
			g.err = nil
		} else if g.err != nil {
			return false
		}

		// After error processing we should check that we actually got a batch
		if g.currentBatchSize == 0 {
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

func (g *GenericMap[K, V]) String() string {
	return g.m.String()
}
