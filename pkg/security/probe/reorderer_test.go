// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/rand"
)

func TestOrder(t *testing.T) {
	heap := &reOrdererHeap{
		pool: &reOrdererNodePool{},
	}
	metric := ReOrdererMetric{}

	for i := 0; i != 200; i++ {
		n := rand.Int()%254 + 1
		heap.enqueue(0, []byte{byte(n)}, uint64(n), 1, &metric)
	}

	var count int
	var last byte
	heap.dequeue(func(cpu uint64, data []byte) {
		count++
		if last > 0 {
			assert.GreaterOrEqual(t, data[0], last)
		}
		last = data[0]
	}, 1, &metric)

	assert.Equal(t, 200, count)
}

func TestOrderRetention(t *testing.T) {
	heap := &reOrdererHeap{
		pool: &reOrdererNodePool{},
	}
	metric := ReOrdererMetric{}

	for i := 0; i != 90; i++ {
		heap.enqueue(0, []byte{byte(i)}, uint64(i), uint64(i/30+1), &metric)
	}

	var count int
	heap.dequeue(func(cpu uint64, data []byte) { count++ }, 1, &metric)
	assert.Equal(t, 30, count)
	heap.dequeue(func(cpu uint64, data []byte) { count++ }, 2, &metric)
	assert.Equal(t, 60, count)
	heap.dequeue(func(cpu uint64, data []byte) { count++ }, 3, &metric)
	assert.Equal(t, 90, count)
}

func TestOrderGeneration(t *testing.T) {
	heap := &reOrdererHeap{
		pool: &reOrdererNodePool{},
	}
	metric := ReOrdererMetric{}

	heap.enqueue(0, []byte{byte(10)}, uint64(10), uint64(1), &metric)
	heap.enqueue(0, []byte{byte(1)}, uint64(1), uint64(2), &metric)

	var data []byte
	heap.dequeue(func(c uint64, d []byte) { data = d }, 1, &metric)
	assert.Equal(t, 1, int(data[0]))

	heap.dequeue(func(c uint64, d []byte) { data = d }, 2, &metric)
	assert.Equal(t, 10, int(data[0]))
}

func TestOrderRate(t *testing.T) {
	var event []byte
	rate := 1 * time.Second
	retention := 5

	var lock sync.RWMutex

	reOrderer := NewReOrderer(func(cpu uint64, data []byte) {
		lock.Lock()
		event = append(event, data[2])
		lock.Unlock()
	},
		func(data []byte) (uint64, uint64, error) {
			return uint64(data[0]), uint64(data[1]), nil
		},
		ReOrdererOpts{
			QueueSize:  100,
			Rate:       rate,
			Retention:  uint64(retention),
			MetricRate: 200 * time.Millisecond,
		})

	ctx, cancel := context.WithCancel(context.Background())
	go reOrderer.Start(ctx)

	var e uint8
	for i := 0; i != 10; i++ {
		reOrderer.HandleEvent(0, []byte{0, byte(i + 1), e}, nil, nil)
		e++
	}

	lock.RLock()
	assert.Zero(t, event)
	lock.RUnlock()

	time.Sleep(100 * time.Millisecond)

	lock.RLock()
	assert.Zero(t, event)
	lock.RUnlock()

	time.Sleep(rate * time.Duration(retention))

	// should now get the elements
	lock.RLock()
	assert.Equal(t, event, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	lock.RUnlock()

	cancel()
}
