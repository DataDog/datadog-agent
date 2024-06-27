// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packets

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func countPoolSize(p *PoolManager[Packet]) int {

	i := 0
	p.refs.Range(func(key, value interface{}) bool {
		i++

		return true
	})

	return i
}

func TestPoolManager(t *testing.T) {

	pool := NewPool(1024)
	manager := NewPoolManager[Packet](pool)

	// passthru mode by default
	assert.True(t, manager.IsPassthru())

	packet := manager.Get()
	manager.Put(packet)
	assert.Equal(t, 0, countPoolSize(manager))

	// passthru mode disabled
	manager.SetPassthru(false)
	assert.False(t, manager.IsPassthru())
	packet = manager.Get()
	manager.Put(packet)
	assert.Equal(t, 1, countPoolSize(manager))
	// second put should remove it from reference tracker and return buffer to pool
	manager.Put(packet)
	assert.Equal(t, 0, countPoolSize(manager))

	for i := 0; i < 10; i++ {
		packet = manager.Get()
		manager.Put(packet)
	}
	assert.Equal(t, 10, countPoolSize(manager))
	manager.Flush()
	assert.Equal(t, 0, countPoolSize(manager))

}

func BenchmarkPoolManagerPassthru(b *testing.B) {
	pool := NewPool(1024)
	manager := NewPoolManager[Packet](pool)

	for i := 0; i < b.N; i++ {
		packet := pool.Get()
		manager.Put(packet)
	}
}

func BenchmarkPoolManagerNoPassthru(b *testing.B) {
	pool := NewPool(1024)
	manager := NewPoolManager[Packet](pool)

	for i := 0; i < b.N; i++ {
		packet := pool.Get()
		manager.Put(packet)
		manager.Put(packet)
	}
}

func BenchmarkSyncPool(b *testing.B) {
	pool := NewPool(1024)

	for i := 0; i < b.N; i++ {
		packet := pool.Get()
		pool.Put(packet)
	}

}
