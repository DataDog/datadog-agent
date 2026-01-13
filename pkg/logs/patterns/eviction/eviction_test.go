// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eviction

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockEvictable implements the Evictable interface for testing
type mockEvictable struct {
	id           int
	frequency    float64
	createdAt    time.Time
	lastAccessAt time.Time
	bytes        int64
}

func (m *mockEvictable) GetFrequency() float64 {
	return m.frequency
}

func (m *mockEvictable) GetCreatedAt() time.Time {
	return m.createdAt
}

func (m *mockEvictable) GetLastAccessAt() time.Time {
	return m.lastAccessAt
}

func (m *mockEvictable) EstimatedBytes() int64 {
	return m.bytes
}

// mockCollection implements the EvictableCollection interface for testing
type mockCollection struct {
	items []*mockEvictable
}

func (m *mockCollection) CollectEvictables() []Evictable {
	result := make([]Evictable, len(m.items))
	for i, item := range m.items {
		result[i] = item
	}
	return result
}

func (m *mockCollection) RemoveEvictable(item Evictable) {
	mockItem := item.(*mockEvictable)
	for i, existing := range m.items {
		if existing.id == mockItem.id {
			m.items = append(m.items[:i], m.items[i+1:]...)
			return
		}
	}
}

// TestEvictLowestScoring_Basic tests basic eviction by count
func TestEvictLowestScoring_Basic(t *testing.T) {
	now := time.Now()

	collection := &mockCollection{
		items: []*mockEvictable{
			{id: 1, frequency: 100, createdAt: now.Add(-10 * time.Minute), lastAccessAt: now, bytes: 100},
			{id: 2, frequency: 50, createdAt: now.Add(-20 * time.Minute), lastAccessAt: now.Add(-15 * time.Minute), bytes: 150},
			{id: 3, frequency: 200, createdAt: now.Add(-5 * time.Minute), lastAccessAt: now, bytes: 200},
			{id: 4, frequency: 10, createdAt: now.Add(-30 * time.Minute), lastAccessAt: now.Add(-25 * time.Minute), bytes: 50},
			{id: 5, frequency: 150, createdAt: now.Add(-15 * time.Minute), lastAccessAt: now.Add(-5 * time.Minute), bytes: 120},
		},
	}

	evicted := EvictLowestScoring(collection, 2, 0.5)

	// Should evict 2 items with lowest scores
	require.Len(t, evicted, 2, "should evict exactly 2 items")
	assert.Len(t, collection.items, 3, "collection should have 3 items remaining")

	// Item 4 should definitely be evicted (lowest frequency, oldest)
	evictedIDs := []int{evicted[0].(*mockEvictable).id, evicted[1].(*mockEvictable).id}
	assert.Contains(t, evictedIDs, 4, "item 4 should be evicted (lowest score)")
}

// TestEvictLowestScoring_EvictAll tests evicting more items than exist
func TestEvictLowestScoring_EvictAll(t *testing.T) {
	now := time.Now()

	collection := &mockCollection{
		items: []*mockEvictable{
			{id: 1, frequency: 100, createdAt: now, lastAccessAt: now, bytes: 100},
			{id: 2, frequency: 50, createdAt: now, lastAccessAt: now, bytes: 150},
		},
	}

	evicted := EvictLowestScoring(collection, 10, 0.5)

	assert.Len(t, evicted, 2, "should evict only available items")
	assert.Empty(t, collection.items, "collection should be empty")
}

// TestEvictLowestScoring_ZeroOrNegative tests edge cases
func TestEvictLowestScoring_ZeroOrNegative(t *testing.T) {
	now := time.Now()

	collection := &mockCollection{
		items: []*mockEvictable{
			{id: 1, frequency: 100, createdAt: now, lastAccessAt: now, bytes: 100},
		},
	}

	tests := []struct {
		name       string
		numToEvict int
	}{
		{"zero items", 0},
		{"negative items", -5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evicted := EvictLowestScoring(collection, tt.numToEvict, 0.5)
			assert.Nil(t, evicted, "should return nil")
			assert.Len(t, collection.items, 1, "collection should be unchanged")
		})
	}
}

// TestEvictLowestScoring_EmptyCollection tests evicting from empty collection
func TestEvictLowestScoring_EmptyCollection(t *testing.T) {
	collection := &mockCollection{items: []*mockEvictable{}}

	evicted := EvictLowestScoring(collection, 5, 0.5)

	assert.Empty(t, evicted, "should return empty slice")
}

// TestEvictLowestScoring_DecayFactor tests that decay factor affects eviction order
func TestEvictLowestScoring_DecayFactor(t *testing.T) {
	now := time.Now()

	// Create items where decay factor matters
	collection := &mockCollection{
		items: []*mockEvictable{
			{id: 1, frequency: 1000, createdAt: now.Add(-24 * time.Hour), lastAccessAt: now.Add(-24 * time.Hour), bytes: 100}, // Old, high freq
			{id: 2, frequency: 50, createdAt: now.Add(-1 * time.Minute), lastAccessAt: now, bytes: 100},                       // New, low freq
		},
	}

	// With high decay (0.8), older items get penalized more
	// Item 2 has low frequency and should still score lower despite being newer
	evicted := EvictLowestScoring(collection, 1, 0.8)

	require.Len(t, evicted, 1)
	// Item 2 (new but very low freq) should be evicted
	assert.Equal(t, 2, evicted[0].(*mockEvictable).id, "low frequency item should be evicted")
}

// TestEvictToMemoryTarget_Basic tests memory-based eviction
func TestEvictToMemoryTarget_Basic(t *testing.T) {
	now := time.Now()

	collection := &mockCollection{
		items: []*mockEvictable{
			{id: 1, frequency: 100, createdAt: now, lastAccessAt: now, bytes: 100},
			{id: 2, frequency: 50, createdAt: now.Add(-10 * time.Minute), lastAccessAt: now.Add(-5 * time.Minute), bytes: 150},
			{id: 3, frequency: 200, createdAt: now, lastAccessAt: now, bytes: 200},
			{id: 4, frequency: 10, createdAt: now.Add(-20 * time.Minute), lastAccessAt: now.Add(-15 * time.Minute), bytes: 50},
		},
	}

	// Need to free 200 bytes
	evicted := EvictToMemoryTarget(collection, 200, 0.5)

	// Calculate total bytes freed
	totalFreed := int64(0)
	for _, item := range evicted {
		totalFreed += item.EstimatedBytes()
	}

	assert.GreaterOrEqual(t, totalFreed, int64(200), "should free at least target bytes")
	assert.NotEmpty(t, evicted, "should evict some items")

	// Item 4 should be evicted first (lowest score)
	assert.Equal(t, 4, evicted[0].(*mockEvictable).id, "should evict lowest scoring item first")
}

// TestEvictToMemoryTarget_ExactTarget tests hitting exact memory target
func TestEvictToMemoryTarget_ExactTarget(t *testing.T) {
	now := time.Now()

	collection := &mockCollection{
		items: []*mockEvictable{
			{id: 1, frequency: 100, createdAt: now, lastAccessAt: now, bytes: 100},
			{id: 2, frequency: 50, createdAt: now.Add(-10 * time.Minute), lastAccessAt: now, bytes: 100},
			{id: 3, frequency: 10, createdAt: now.Add(-20 * time.Minute), lastAccessAt: now, bytes: 100},
		},
	}

	// Need to free exactly 100 bytes
	evicted := EvictToMemoryTarget(collection, 100, 0.5)

	totalFreed := int64(0)
	for _, item := range evicted {
		totalFreed += item.EstimatedBytes()
	}

	assert.GreaterOrEqual(t, totalFreed, int64(100), "should free at least 100 bytes")
	assert.Len(t, collection.items, 2, "should have 2 items remaining")
}

// TestEvictToMemoryTarget_EvictAll tests evicting all items for memory
func TestEvictToMemoryTarget_EvictAll(t *testing.T) {
	now := time.Now()

	collection := &mockCollection{
		items: []*mockEvictable{
			{id: 1, frequency: 100, createdAt: now, lastAccessAt: now, bytes: 100},
			{id: 2, frequency: 50, createdAt: now, lastAccessAt: now, bytes: 150},
		},
	}

	// Need more bytes than available
	evicted := EvictToMemoryTarget(collection, 10000, 0.5)

	assert.Len(t, evicted, 2, "should evict all items")
	assert.Empty(t, collection.items, "collection should be empty")

	totalFreed := int64(0)
	for _, item := range evicted {
		totalFreed += item.EstimatedBytes()
	}
	assert.Equal(t, int64(250), totalFreed, "should free all available bytes")
}

// TestEvictToMemoryTarget_ZeroOrNegative tests edge cases
func TestEvictToMemoryTarget_ZeroOrNegative(t *testing.T) {
	now := time.Now()

	collection := &mockCollection{
		items: []*mockEvictable{
			{id: 1, frequency: 100, createdAt: now, lastAccessAt: now, bytes: 100},
		},
	}

	tests := []struct {
		name       string
		targetByte int64
	}{
		{"zero bytes", 0},
		{"negative bytes", -100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evicted := EvictToMemoryTarget(collection, tt.targetByte, 0.5)
			assert.Nil(t, evicted, "should return nil")
			assert.Len(t, collection.items, 1, "collection should be unchanged")
		})
	}
}

// TestEvictToMemoryTarget_EmptyCollection tests evicting from empty collection
func TestEvictToMemoryTarget_EmptyCollection(t *testing.T) {
	collection := &mockCollection{items: []*mockEvictable{}}

	evicted := EvictToMemoryTarget(collection, 1000, 0.5)

	assert.Empty(t, evicted, "should return empty slice")
}

// TestEvictToMemoryTarget_IncrementalEviction tests that items are evicted one by one in score order
func TestEvictToMemoryTarget_IncrementalEviction(t *testing.T) {
	now := time.Now()

	collection := &mockCollection{
		items: []*mockEvictable{
			{id: 1, frequency: 100, createdAt: now, lastAccessAt: now, bytes: 50},                                              // High score
			{id: 2, frequency: 50, createdAt: now.Add(-10 * time.Minute), lastAccessAt: now.Add(-5 * time.Minute), bytes: 50},  // Medium score
			{id: 3, frequency: 10, createdAt: now.Add(-30 * time.Minute), lastAccessAt: now.Add(-25 * time.Minute), bytes: 50}, // Low score
		},
	}

	// Need to free 60 bytes (should evict 2 items)
	evicted := EvictToMemoryTarget(collection, 60, 0.5)

	require.Len(t, evicted, 2, "should evict 2 items to reach target")
	assert.Equal(t, 3, evicted[0].(*mockEvictable).id, "should evict lowest scoring item first")
	assert.Equal(t, 2, evicted[1].(*mockEvictable).id, "should evict second lowest scoring item")
	assert.Len(t, collection.items, 1, "should have 1 item remaining")
	assert.Equal(t, 1, collection.items[0].id, "highest scoring item should remain")
}

// TestEvictToMemoryTarget_LargeItems tests eviction with varying item sizes
func TestEvictToMemoryTarget_LargeItems(t *testing.T) {
	now := time.Now()

	collection := &mockCollection{
		items: []*mockEvictable{
			{id: 1, frequency: 10, createdAt: now.Add(-30 * time.Minute), lastAccessAt: now.Add(-25 * time.Minute), bytes: 1000}, // Low score, large
			{id: 2, frequency: 100, createdAt: now, lastAccessAt: now, bytes: 10},                                                // High score, small
		},
	}

	// Need to free 500 bytes - should evict item 1 even though it's large
	evicted := EvictToMemoryTarget(collection, 500, 0.5)

	require.Len(t, evicted, 1, "should evict 1 item")
	assert.Equal(t, 1, evicted[0].(*mockEvictable).id, "should evict large item with lowest score")

	totalFreed := evicted[0].EstimatedBytes()
	assert.Equal(t, int64(1000), totalFreed, "should free 1000 bytes")
}
