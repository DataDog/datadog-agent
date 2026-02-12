// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"hash/fnv"
	"math/rand"
	"sync"
)

// StableBloomFilter handles unbounded streams by probabilistically evicting old entries.
// Unlike classic Bloom filters that fill up, this maintains a stable false positive rate.
// Based on Deng & Rafiei (2006) - "Approximately Detecting Duplicates for Streaming Data using Stable Bloom Filters"
type StableBloomFilter struct {
	cells     []uint8 // Each cell is a counter (0-max)
	numCells  uint32
	numHashes uint32
	max       uint8  // Max counter value (e.g., 3)
	p         uint32 // Number of cells to decrement on each add (controls eviction rate)
	mu        sync.RWMutex
	rng       *rand.Rand
}

// NewStableBloomFilter creates a new Stable Bloom Filter.
// - numCells: size of the filter (larger = lower FP rate)
// - numHashes: number of hash functions (typically 3-5)
// - max: maximum counter value (typically 3)
// - p: cells to decrement per add (controls memory/accuracy tradeoff)
func NewStableBloomFilter(numCells, numHashes uint32, max uint8, p uint32) *StableBloomFilter {
	return &StableBloomFilter{
		cells:     make([]uint8, numCells),
		numCells:  numCells,
		numHashes: numHashes,
		max:       max,
		p:         p,
		rng:       rand.New(rand.NewSource(42)),
	}
}

// hash returns k hash values for the given key using double hashing
func (f *StableBloomFilter) hash(key []byte) []uint32 {
	hashes := make([]uint32, f.numHashes)
	h1 := fnv.New32a()
	h1.Write(key)
	hash1 := h1.Sum32()

	h2 := fnv.New32()
	h2.Write(key)
	hash2 := h2.Sum32()

	// Double hashing: h(i) = h1 + i*h2
	for i := uint32(0); i < f.numHashes; i++ {
		hashes[i] = (hash1 + i*hash2) % f.numCells
	}
	return hashes
}

// Add inserts an element and randomly decrements p cells (eviction)
func (f *StableBloomFilter) Add(key []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Decrement p random cells (stable eviction)
	for i := uint32(0); i < f.p; i++ {
		idx := f.rng.Uint32() % f.numCells
		if f.cells[idx] > 0 {
			f.cells[idx]--
		}
	}

	// Set cells for this key to max
	for _, idx := range f.hash(key) {
		f.cells[idx] = f.max
	}
}

// Test checks if an element might be in the filter
func (f *StableBloomFilter) Test(key []byte) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	for _, idx := range f.hash(key) {
		if f.cells[idx] == 0 {
			return false // Definitely not present
		}
	}
	return true // Possibly present
}

// TestAndAdd atomically tests and adds
func (f *StableBloomFilter) TestAndAdd(key []byte) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Test first
	present := true
	for _, idx := range f.hash(key) {
		if f.cells[idx] == 0 {
			present = false
			break
		}
	}

	// Decrement p random cells
	for i := uint32(0); i < f.p; i++ {
		idx := f.rng.Uint32() % f.numCells
		if f.cells[idx] > 0 {
			f.cells[idx]--
		}
	}

	// Add
	for _, idx := range f.hash(key) {
		f.cells[idx] = f.max
	}

	return present
}

// AnomalyDedupConfig configures the AnomalyDeduplicator.
type AnomalyDedupConfig struct {
	// NumCells is the size of the bloom filter (larger = lower FP rate)
	// Default: 100000
	NumCells uint32

	// BucketSizeSeconds is the time granularity for deduplication.
	// Anomalies from the same source within the same bucket are considered duplicates.
	// Default: 5 seconds
	BucketSizeSeconds int64
}

// DefaultAnomalyDedupConfig returns default configuration.
func DefaultAnomalyDedupConfig() AnomalyDedupConfig {
	return AnomalyDedupConfig{
		NumCells:          100000,
		BucketSizeSeconds: 5,
	}
}

// AnomalyDeduplicator wraps StableBloomFilter for anomaly deduplication.
// It prevents the same source from generating duplicate anomalies within a time bucket.
type AnomalyDeduplicator struct {
	filter            *StableBloomFilter
	bucketSizeSeconds int64

	// Stats
	totalSeen    int64
	totalDropped int64
	mu           sync.RWMutex
}

// NewAnomalyDeduplicator creates a new deduplicator with the given config.
func NewAnomalyDeduplicator(config AnomalyDedupConfig) *AnomalyDeduplicator {
	if config.NumCells == 0 {
		config.NumCells = 100000
	}
	if config.BucketSizeSeconds == 0 {
		config.BucketSizeSeconds = 5
	}
	return &AnomalyDeduplicator{
		// numCells, numHashes=3, max=3, p=1
		filter:            NewStableBloomFilter(config.NumCells, 3, 3, 1),
		bucketSizeSeconds: config.BucketSizeSeconds,
	}
}

// ShouldProcess returns true if this anomaly should be processed (not a duplicate).
// It also adds the anomaly to the filter, so subsequent calls with the same
// seriesID+time bucket will return false.
func (d *AnomalyDeduplicator) ShouldProcess(seriesID string, timestamp int64) bool {
	key := fmt.Sprintf("%s|%d", seriesID, timestamp/d.bucketSizeSeconds)

	d.mu.Lock()
	d.totalSeen++
	d.mu.Unlock()

	isDuplicate := d.filter.TestAndAdd([]byte(key))

	if isDuplicate {
		d.mu.Lock()
		d.totalDropped++
		d.mu.Unlock()
		return false
	}
	return true
}

// Stats returns deduplication statistics.
func (d *AnomalyDeduplicator) Stats() map[string]interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()

	dropRate := 0.0
	if d.totalSeen > 0 {
		dropRate = float64(d.totalDropped) / float64(d.totalSeen)
	}

	return map[string]interface{}{
		"total_seen":          d.totalSeen,
		"total_dropped":       d.totalDropped,
		"drop_rate":           dropRate,
		"bucket_size_seconds": d.bucketSizeSeconds,
	}
}

// Reset clears the deduplicator state.
func (d *AnomalyDeduplicator) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.filter = NewStableBloomFilter(uint32(len(d.filter.cells)), d.filter.numHashes, d.filter.max, d.filter.p)
	d.totalSeen = 0
	d.totalDropped = 0
}
