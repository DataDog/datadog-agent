// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package sync is utilities for synchronization
package sync

import (
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/funcs"
)

// PoolReleaser is interface that wraps a sync.Pool Put function
type PoolReleaser[K any] interface {
	Put(*K)
}

// PoolGetter is interface that wraps a sync.Pool Get function
type PoolGetter[K any] interface {
	Get() *K
}

// Pool is a combination interface of PoolGetter and PoolReleaser
type Pool[K any] interface {
	PoolGetter[K]
	PoolReleaser[K]
}

// TypedPool is a type-safe version of sync.Pool
type TypedPool[K any] struct {
	p sync.Pool
}

// NewDefaultTypedPool creates a TypedPool using the default `new` function to create instances of K
func NewDefaultTypedPool[K any]() *TypedPool[K] {
	return NewTypedPool(func() *K {
		return new(K)
	})
}

// NewSlicePool creates a TypedPool using `make` to create slices of specified size and capacity for instances of []K
func NewSlicePool[K any](size int, capacity int) *TypedPool[[]K] {
	return NewTypedPool(func() *[]K {
		s := make([]K, size, capacity)
		return &s
	})
}

// NewTypedPool creates a TypedPool using the provided function to create instances of K
func NewTypedPool[K any](f func() *K) *TypedPool[K] {
	return &TypedPool[K]{
		p: sync.Pool{
			New: func() any {
				return f()
			},
		},
	}
}

// Get wraps sync.Pool.Get in a type-safe way
func (t *TypedPool[K]) Get() *K {
	return t.p.Get().(*K)
}

// Put wraps sync.Pool.Put in a type-safe way
func (t *TypedPool[K]) Put(x *K) {
	t.p.Put(x)
}

// typedPoolWithTelemetry is a TypedPool with telemetry counters
type typedPoolWithTelemetry[K any] struct {
	*TypedPool[K]
	tm *poolTelemetrySimple
}

// poolTelemetry is a struct that contains the global telemetry counters for the pool
type poolTelemetry struct {
	get    telemetry.Counter
	put    telemetry.Counter
	active telemetry.Gauge
}

// getSimpleCounters gets counters with pre-computed tags to reduce unnecessary repeated work
func (p *poolTelemetry) getSimpleCounters(module, name string) *poolTelemetrySimple {
	return &poolTelemetrySimple{
		get:    p.get.WithTags(map[string]string{"module": module, "pool_name": name}),
		put:    p.put.WithTags(map[string]string{"module": module, "pool_name": name}),
		active: p.active.WithTags(map[string]string{"module": module, "pool_name": name}),
	}
}

// poolTelemetrySimple is a struct that contains the simple telemetry counters for the pool
type poolTelemetrySimple struct {
	get    telemetry.SimpleCounter
	put    telemetry.SimpleCounter
	active telemetry.SimpleGauge
}

func newPoolTelemetry(tm telemetry.Component) *poolTelemetry {
	return &poolTelemetry{
		get:    tm.NewCounter("sync__pool", "get", []string{"module", "pool_name"}, "Number of gets from the sync pool"),
		put:    tm.NewCounter("sync__pool", "put", []string{"module", "pool_name"}, "Number of puts to the sync pool"),
		active: tm.NewGauge("sync__pool", "active", []string{"module", "pool_name"}, "Number of active items in the sync pool"),
	}
}

// globalPoolTelemetry is a memoized function to create a single poolTelemetry instance from the telemetry component
var globalPoolTelemetry = funcs.MemoizeArgNoError(newPoolTelemetry)

// NewDefaultTypedPoolWithTelemetry creates a TypedPool with telemetry using the default `new` function to create instances of K
// module and name are used to identify the pool in the telemetry
func NewDefaultTypedPoolWithTelemetry[K any](tm telemetry.Component, module string, name string) Pool[K] {
	return &typedPoolWithTelemetry[K]{
		TypedPool: NewDefaultTypedPool[K](),
		tm:        globalPoolTelemetry(tm).getSimpleCounters(module, name),
	}
}

func (t *typedPoolWithTelemetry[K]) Get() *K {
	t.tm.get.Inc()
	t.tm.active.Inc()
	return t.TypedPool.Get()
}

func (t *typedPoolWithTelemetry[K]) Put(x *K) {
	t.tm.put.Inc()
	t.tm.active.Dec()
	t.p.Put(x)
}
