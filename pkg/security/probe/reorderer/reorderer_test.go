// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

//go:build linux

package reorderer

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/cilium/ebpf/perf"
	"github.com/stretchr/testify/assert"
)

func TestOrder(t *testing.T) {
	heap := &reOrdererHeap{
		pool: &reOrdererNodePool{},
	}
	metric := ReOrdererMetric{}

	for i := 0; i != 200; i++ {
		n := rand.Int()%254 + 1

		record := perf.Record{
			RawSample: []byte{byte(n)},
		}
		heap.enqueue(&record, uint64(n), 1, &metric)
	}

	var count int
	var last byte
	heap.dequeue(func(record *perf.Record) {
		count++
		if last > 0 {
			assert.GreaterOrEqual(t, record.RawSample[0], last)
		}
		last = record.RawSample[0]
	}, 1, &metric, &ReOrdererOpts{})

	assert.Equal(t, 200, count)
}

func TestOrderRetention(t *testing.T) {
	heap := &reOrdererHeap{
		pool: &reOrdererNodePool{},
	}
	metric := ReOrdererMetric{}

	for i := 0; i != 90; i++ {
		record := perf.Record{
			RawSample: []byte{byte(i)},
		}

		heap.enqueue(&record, uint64(i), uint64(i/30+1), &metric)
	}

	var count int
	heap.dequeue(func(record *perf.Record) { count++ }, 1, &metric, &ReOrdererOpts{})
	assert.Equal(t, 30, count)
	heap.dequeue(func(record *perf.Record) { count++ }, 2, &metric, &ReOrdererOpts{})
	assert.Equal(t, 60, count)
	heap.dequeue(func(record *perf.Record) { count++ }, 3, &metric, &ReOrdererOpts{})
	assert.Equal(t, 90, count)
}

func TestOrderGeneration(t *testing.T) {
	heap := &reOrdererHeap{
		pool: &reOrdererNodePool{},
	}
	metric := ReOrdererMetric{}

	record1 := perf.Record{
		RawSample: []byte{byte(10)},
	}
	heap.enqueue(&record1, uint64(10), uint64(1), &metric)

	record2 := perf.Record{
		RawSample: []byte{byte(1)},
	}
	heap.enqueue(&record2, uint64(1), uint64(2), &metric)

	var data []byte
	heap.dequeue(func(record *perf.Record) { data = record.RawSample }, 1, &metric, &ReOrdererOpts{})
	assert.Equal(t, 1, int(data[0]))

	heap.dequeue(func(record *perf.Record) { data = record.RawSample }, 2, &metric, &ReOrdererOpts{})
	assert.Equal(t, 10, int(data[0]))
}

func TestOrderRate(t *testing.T) {
	var event []byte
	rate := 1 * time.Second
	retention := 5

	var lock sync.RWMutex
	ctx, cancel := context.WithCancel(context.Background())

	reOrderer := NewReOrderer(ctx, func(record *perf.Record) {
		lock.Lock()
		event = append(event, record.RawSample[2])
		lock.Unlock()
	},
		func(record *perf.Record) (QuickInfo, error) {
			return QuickInfo{
				Cpu:       uint64(record.RawSample[0]),
				Timestamp: uint64(record.RawSample[1]),
			}, nil
		},
		ReOrdererOpts{
			QueueSize:  100,
			Rate:       rate,
			Retention:  uint64(retention),
			MetricRate: 200 * time.Millisecond,
		})

	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Add(1)
	go reOrderer.Start(&wg)

	var e uint8
	for i := 0; i != 10; i++ {
		record := perf.Record{
			RawSample: []byte{0, byte(i + 1), e},
		}

		reOrderer.HandleEvent(&record, nil, nil)
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
	assert.Equal(t, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, event)
	lock.RUnlock()

	cancel()
}
