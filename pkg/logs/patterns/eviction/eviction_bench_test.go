// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eviction

import (
	"container/heap"
	"math/rand"
	"testing"
	"time"
)

// benchEvictable implements Evictable for benchmarking with realistic distributions.
type benchEvictable struct {
	id           int
	frequency    float64
	createdAt    time.Time
	lastAccessAt time.Time
	bytes        int64
}

func (b *benchEvictable) GetFrequency() float64      { return b.frequency }
func (b *benchEvictable) GetCreatedAt() time.Time    { return b.createdAt }
func (b *benchEvictable) GetLastAccessAt() time.Time { return b.lastAccessAt }
func (b *benchEvictable) EstimatedBytes() int64      { return b.bytes }

// benchCollection implements EvictableCollection for benchmarking.
type benchCollection struct {
	items []*benchEvictable
}

func (c *benchCollection) CollectEvictables() []Evictable {
	result := make([]Evictable, len(c.items))
	for i, item := range c.items {
		result[i] = item
	}
	return result
}

func (c *benchCollection) RemoveEvictable(item Evictable) {
	benchItem := item.(*benchEvictable)
	for i, existing := range c.items {
		if existing.id == benchItem.id {
			c.items = append(c.items[:i], c.items[i+1:]...)
			return
		}
	}
}

// makeBenchCollection creates a collection with realistic frequency/timestamp distributions.
// Uses a fixed seed for reproducible benchmarks.
func makeBenchCollection(size int, rng *rand.Rand) *benchCollection {
	now := time.Now()
	items := make([]*benchEvictable, size)
	for i := 0; i < size; i++ {
		// Realistic frequency: power-law-ish (few high, many low)
		freq := 1.0 + rng.ExpFloat64()*500
		if freq < 1 {
			freq = 1
		}
		// Age: 0-90 days, skewed toward recent
		ageDays := rng.Float64() * rng.Float64() * 90
		createdAt := now.Add(-time.Duration(ageDays*24) * time.Hour)
		// Last access: between creation and now
		accessDays := rng.Float64() * ageDays
		lastAccessAt := createdAt.Add(time.Duration(accessDays*24) * time.Hour)
		if lastAccessAt.After(now) {
			lastAccessAt = now
		}
		// Bytes: 50-500 typical for pattern/tag entries
		bytes := int64(50 + rng.Intn(450))
		items[i] = &benchEvictable{
			id:           i,
			frequency:    freq,
			createdAt:    createdAt,
			lastAccessAt: lastAccessAt,
			bytes:        bytes,
		}
	}
	return &benchCollection{items: items}
}

// --- EvictLowestScoring by collection size ---

func BenchmarkEvictLowestScoring_Size100_Evict1(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	decayFactor := 0.5
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collection := makeBenchCollection(100, rng)
		EvictLowestScoring(collection, 1, decayFactor)
	}
}

func BenchmarkEvictLowestScoring_Size100_Evict10(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	decayFactor := 0.5
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collection := makeBenchCollection(100, rng)
		EvictLowestScoring(collection, 10, decayFactor)
	}
}

func BenchmarkEvictLowestScoring_Size100_Evict50(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	decayFactor := 0.5
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collection := makeBenchCollection(100, rng)
		EvictLowestScoring(collection, 50, decayFactor)
	}
}

func BenchmarkEvictLowestScoring_Size500_Evict1(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	decayFactor := 0.5
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collection := makeBenchCollection(500, rng)
		EvictLowestScoring(collection, 1, decayFactor)
	}
}

func BenchmarkEvictLowestScoring_Size500_Evict50(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	decayFactor := 0.5
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collection := makeBenchCollection(500, rng)
		EvictLowestScoring(collection, 50, decayFactor)
	}
}

func BenchmarkEvictLowestScoring_Size1000_Evict1(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	decayFactor := 0.5
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collection := makeBenchCollection(1000, rng)
		EvictLowestScoring(collection, 1, decayFactor)
	}
}

func BenchmarkEvictLowestScoring_Size1000_Evict10(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	decayFactor := 0.5
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collection := makeBenchCollection(1000, rng)
		EvictLowestScoring(collection, 10, decayFactor)
	}
}

func BenchmarkEvictLowestScoring_Size1000_Evict100(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	decayFactor := 0.5
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collection := makeBenchCollection(1000, rng)
		EvictLowestScoring(collection, 100, decayFactor)
	}
}

func BenchmarkEvictLowestScoring_Size5000_Evict1(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	decayFactor := 0.5
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collection := makeBenchCollection(5000, rng)
		EvictLowestScoring(collection, 1, decayFactor)
	}
}

func BenchmarkEvictLowestScoring_Size5000_Evict50(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	decayFactor := 0.5
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collection := makeBenchCollection(5000, rng)
		EvictLowestScoring(collection, 50, decayFactor)
	}
}

func BenchmarkEvictLowestScoring_Size5000_Evict100(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	decayFactor := 0.5
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collection := makeBenchCollection(5000, rng)
		EvictLowestScoring(collection, 100, decayFactor)
	}
}

func BenchmarkEvictLowestScoring_Size10000_Evict1(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	decayFactor := 0.5
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collection := makeBenchCollection(10000, rng)
		EvictLowestScoring(collection, 1, decayFactor)
	}
}

func BenchmarkEvictLowestScoring_Size10000_Evict50(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	decayFactor := 0.5
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collection := makeBenchCollection(10000, rng)
		EvictLowestScoring(collection, 50, decayFactor)
	}
}

func BenchmarkEvictLowestScoring_Size10000_Evict100(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	decayFactor := 0.5
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collection := makeBenchCollection(10000, rng)
		EvictLowestScoring(collection, 100, decayFactor)
	}
}

// --- CalculateScore in isolation ---

func BenchmarkCalculateScore(b *testing.B) {
	now := time.Now()
	frequency := 100.0
	createdAt := now.Add(-7 * 24 * time.Hour)
	lastAccessAt := now.Add(-1 * time.Hour)
	decayFactor := 0.5
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalculateScore(frequency, createdAt, lastAccessAt, now, decayFactor)
	}
}

func BenchmarkCalculateScore_Batch1000(b *testing.B) {
	now := time.Now()
	rng := rand.New(rand.NewSource(42))
	decayFactor := 0.5
	// Pre-build 1000 items
	items := make([]struct {
		freq    float64
		created time.Time
		access  time.Time
	}, 1000)
	for i := range items {
		freq := 1.0 + rng.ExpFloat64()*500
		if freq < 1 {
			freq = 1
		}
		ageDays := rng.Float64() * 90
		created := now.Add(-time.Duration(ageDays*24) * time.Hour)
		accessDays := rng.Float64() * ageDays
		access := created.Add(time.Duration(accessDays*24) * time.Hour)
		if access.After(now) {
			access = now
		}
		items[i] = struct {
			freq    float64
			created time.Time
			access  time.Time
		}{freq, created, access}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range items {
			CalculateScore(items[j].freq, items[j].created, items[j].access, now, decayFactor)
		}
	}
}

// --- Heap operations in isolation (simulates heap.Init + K pops) ---

func BenchmarkHeapInitAndPop1_Size100(b *testing.B) {
	benchmarkHeapInitAndPop(b, 100, 1)
}

func BenchmarkHeapInitAndPop10_Size100(b *testing.B) {
	benchmarkHeapInitAndPop(b, 100, 10)
}

func BenchmarkHeapInitAndPop50_Size1000(b *testing.B) {
	benchmarkHeapInitAndPop(b, 1000, 50)
}

func BenchmarkHeapInitAndPop100_Size5000(b *testing.B) {
	benchmarkHeapInitAndPop(b, 5000, 100)
}

func benchmarkHeapInitAndPop(b *testing.B, size, numPop int) {
	now := time.Now()
	rng := rand.New(rand.NewSource(42))
	decayFactor := 0.5
	// Pre-build heap items (scores only - no Evictable)
	items := make([]heapItem, size)
	for i := 0; i < size; i++ {
		freq := 1.0 + rng.ExpFloat64()*500
		if freq < 1 {
			freq = 1
		}
		ageDays := rng.Float64() * rng.Float64() * 90
		created := now.Add(-time.Duration(ageDays*24) * time.Hour)
		accessDays := rng.Float64() * ageDays
		access := created.Add(time.Duration(accessDays*24) * time.Hour)
		if access.After(now) {
			access = now
		}
		score := CalculateScore(freq, created, access, now, decayFactor)
		items[i] = heapItem{item: &benchEvictable{id: i}, score: score}
	}
	h := &evictionHeap{items: items}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Copy items for each run (heap is destructive)
		copied := make([]heapItem, size)
		copy(copied, items)
		h.items = copied
		heap.Init(h)
		for j := 0; j < numPop && h.Len() > 0; j++ {
			heap.Pop(h)
		}
	}
}

// --- time.Now() cost ---

func BenchmarkTimeNow(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = time.Now()
	}
}
