// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packets

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func countPoolSize(p *PoolManager) int {

	i := 0
	p.refs.Range(func(key, value interface{}) bool {
		i++

		return true
	})

	return i
}

func TestPoolManager(t *testing.T) {
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	packetsTelemetryStore := NewTelemetryStore(nil, telemetryComponent)
	pool := NewPool(1024, packetsTelemetryStore)
	manager := NewPoolManager(pool)

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
	telemetryComponent := fxutil.Test[telemetry.Component](b, telemetryimpl.MockModule())
	packetsTelemetryStore := NewTelemetryStore(nil, telemetryComponent)
	pool := NewPool(1024, packetsTelemetryStore)
	manager := NewPoolManager(pool)

	for i := 0; i < b.N; i++ {
		packet := pool.Get()
		manager.Put(packet)
	}
}

func BenchmarkPoolManagerNoPassthru(b *testing.B) {
	telemetryComponent := fxutil.Test[telemetry.Component](b, telemetryimpl.MockModule())
	packetsTelemetryStore := NewTelemetryStore(nil, telemetryComponent)
	pool := NewPool(1024, packetsTelemetryStore)
	manager := NewPoolManager(pool)

	for i := 0; i < b.N; i++ {
		packet := pool.Get()
		manager.Put(packet)
		manager.Put(packet)
	}
}

func BenchmarkSyncPool(b *testing.B) {
	telemetryComponent := fxutil.Test[telemetry.Component](b, telemetryimpl.MockModule())
	packetsTelemetryStore := NewTelemetryStore(nil, telemetryComponent)
	pool := NewPool(1024, packetsTelemetryStore)

	for i := 0; i < b.N; i++ {
		packet := pool.Get()
		pool.Put(packet)
	}

}
