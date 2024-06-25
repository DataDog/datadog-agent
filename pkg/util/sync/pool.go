// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package sync

import "sync"

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
