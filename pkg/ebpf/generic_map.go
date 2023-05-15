// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpf

import (
	"fmt"
	"unsafe"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
)

type GenericMap[K interface{}, V interface{}] struct {
	m *ebpf.Map
}

func Map[K interface{}, V interface{}](m *ebpf.Map) *GenericMap[K, V] {
	return &GenericMap[K, V]{
		m: m,
	}
}

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

func (g *GenericMap[K, V]) Map() *ebpf.Map {
	return g.m
}

func (g *GenericMap[K, V]) Iterate() *GenericMapIterator[K, V] {
	return &GenericMapIterator[K, V]{
		it: g.m.Iterate(),
	}
}

func (g *GenericMap[K, V]) Put(key *K, value *V) error {
	return g.Update(key, value, ebpf.UpdateAny)
}

func (g *GenericMap[K, V]) Update(key *K, value *V, flags ebpf.MapUpdateFlags) error {
	return g.m.Update(unsafe.Pointer(key), unsafe.Pointer(value), flags)
}

func (g *GenericMap[K, V]) Lookup(key *K, valueOut *V) error {
	return g.m.Lookup(unsafe.Pointer(key), unsafe.Pointer(valueOut))
}

func (g *GenericMap[K, V]) Delete(key *K) error {
	return g.m.Delete(unsafe.Pointer(key))
}

type GenericMapIterator[K interface{}, V interface{}] struct {
	it *ebpf.MapIterator
}

func (g *GenericMapIterator[K, V]) Next(key *K, value *V) bool {
	return g.it.Next(unsafe.Pointer(key), unsafe.Pointer(value))
}

func (g *GenericMapIterator[K, V]) Err() error {
	return g.it.Err()
}
