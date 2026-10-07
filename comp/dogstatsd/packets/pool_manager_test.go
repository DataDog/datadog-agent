// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packets

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	mocktelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// recyclingPool makes premature and duplicate returns observable without relying
// on sync.Pool's nondeterministic reuse (especially under the race detector).
type recyclingPool struct {
	sync.Mutex
	available []*Packet
	returns   map[*Packet]int
}

func (p *recyclingPool) Get() *Packet {
	p.Lock()
	defer p.Unlock()
	if len(p.available) == 0 {
		return &Packet{Buffer: make([]byte, 32)}
	}
	x := p.available[len(p.available)-1]
	p.available = p.available[:len(p.available)-1]
	return x
}

func (p *recyclingPool) Put(x *Packet) {
	p.Lock()
	defer p.Unlock()
	if p.returns == nil {
		p.returns = make(map[*Packet]int)
	}
	p.returns[x]++
	p.available = append(p.available, x)
}

func TestPoolManager(t *testing.T) {
	pool := &recyclingPool{}
	manager := NewPoolManager[Packet](pool)
	packet := manager.Get()
	manager.Put(packet)
	require.Same(t, packet, manager.Get())
	require.Zero(t, manager.Count())

	manager.Retain(packet)
	manager.Put(packet)
	require.Equal(t, 1, manager.Count())
	other := manager.Get()
	require.NotSame(t, packet, other, "capture still owns the original buffer")
	manager.Put(other)
	require.Equal(t, 1, pool.returns[other], "unshared packets return normally during capture")
	manager.Put(packet)
	require.Zero(t, manager.Count())
	require.Same(t, packet, manager.Get())
	require.Equal(t, 2, pool.returns[packet]) // once before capture, once after both owners
	manager.Put(nil)
	manager.Retain(nil)
	require.Zero(t, manager.Count())
}

func TestPoolManagerConcurrentRelease(t *testing.T) {
	pool := &recyclingPool{}
	manager := NewPoolManager[Packet](pool)
	var wg sync.WaitGroup
	start := make(chan struct{})
	const count = 1000
	for range count {
		packet := manager.Get()
		manager.Retain(packet)
		manager.Retain(packet)
		for range 3 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				manager.Put(packet)
			}()
		}
	}
	close(start)
	wg.Wait()
	require.Zero(t, manager.Count())
	require.Len(t, pool.returns, count)
	for _, n := range pool.returns {
		require.Equal(t, 1, n, "each object must return exactly once")
	}
}

func BenchmarkPoolManagerPassthru(b *testing.B) {
	telemetryComponent := fxutil.Test[telemetry.Component](b, mocktelemetry.Module())
	packetsTelemetryStore := NewTelemetryStore(nil, telemetryComponent)
	pool := NewPool(configmock.New(b), 1024, packetsTelemetryStore)
	manager := NewPoolManager[Packet](pool)

	for i := 0; i < b.N; i++ {
		packet := pool.Get()
		manager.Put(packet)
	}
}
func BenchmarkPoolManagerRetained(b *testing.B) {
	telemetryComponent := fxutil.Test[telemetry.Component](b, mocktelemetry.Module())
	packetsTelemetryStore := NewTelemetryStore(nil, telemetryComponent)
	pool := NewPool(configmock.New(b), 1024, packetsTelemetryStore)
	manager := NewPoolManager[Packet](pool)

	for i := 0; i < b.N; i++ {
		packet := pool.Get()
		manager.Retain(packet)
		manager.Put(packet)
		manager.Put(packet)
	}
}

func BenchmarkSyncPool(b *testing.B) {
	telemetryComponent := fxutil.Test[telemetry.Component](b, mocktelemetry.Module())
	packetsTelemetryStore := NewTelemetryStore(nil, telemetryComponent)
	pool := NewPool(configmock.New(b), 1024, packetsTelemetryStore)

	for i := 0; i < b.N; i++ {
		packet := pool.Get()
		pool.Put(packet)
	}

}
