package api

import (
	"math"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWeightedQueue(t *testing.T) {
	q := NewWeightedQueue(10, math.MaxInt64)
	exit := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		for i := 0; i < 10; i++ {
			item, ok := q.Poll(exit)

			assert.True(t, ok)
			assert.Equal(t, "item", item.Type())
			assert.Equal(t, int64(i), item.Weight())
		}
	}()

	for i := 0; i < 10; i++ {
		q.Add(newItem("item", int64(i)))
	}

	wg.Wait()
}

func TestWeightedQueuePollInterrupt(t *testing.T) {
	q := NewWeightedQueue(3, math.MaxInt64)
	exit := make(chan struct{})

	go func() {
		time.Sleep(500 * time.Millisecond)
		close(exit)
	}()

	item, ok := q.Poll(exit)
	assert.False(t, ok)
	assert.Nil(t, item)
}

func TestWeightedQueuePollBlocking(t *testing.T) {
	q := NewWeightedQueue(3, math.MaxInt64)
	exit := make(chan struct{})

	go func() {
		time.Sleep(500 * time.Millisecond)
		q.Add(newItem("item", 1))
	}()

	item, ok := q.Poll(exit)
	assert.True(t, ok)
	assert.Equal(t, "item", item.Type())
	assert.Equal(t, int64(1), item.Weight())
}

func TestWeightedQueueItemsEvicted(t *testing.T) {
	q := NewWeightedQueue(3, math.MaxInt64)
	exit := make(chan struct{})

	q.Add(newItem("item", 1))
	q.Add(newItem("item", 2))
	q.Add(newItem("item", 3))
	q.Add(newItem("item", 4))

	item, ok := q.Poll(exit)
	assert.True(t, ok)
	assert.Equal(t, "item", item.Type())
	assert.Equal(t, int64(2), item.Weight())

	item, ok = q.Poll(exit)
	assert.True(t, ok)
	assert.Equal(t, int64(3), item.Weight())

	item, ok = q.Poll(exit)
	assert.True(t, ok)
	assert.Equal(t, int64(4), item.Weight())

	assert.Equal(t, 0, q.Len())
	assert.Equal(t, int64(0), q.Weight())
}

func TestWeightedQueueItemsEvictedByType(t *testing.T) {
	q := NewWeightedQueue(3, math.MaxInt64)
	exit := make(chan struct{})

	q.Add(newItem("item1", 1))
	q.Add(newItem("item2", 2))
	q.Add(newItem("item1", 3))
	q.Add(newItem("item2", 4))

	item, ok := q.Poll(exit)
	assert.True(t, ok)
	assert.Equal(t, "item1", item.Type())
	assert.Equal(t, int64(1), item.Weight())

	item, ok = q.Poll(exit)
	assert.True(t, ok)
	assert.Equal(t, "item1", item.Type())
	assert.Equal(t, int64(3), item.Weight())

	item, ok = q.Poll(exit)
	assert.True(t, ok)
	assert.Equal(t, "item2", item.Type())
	assert.Equal(t, int64(4), item.Weight())

	assert.Equal(t, 0, q.Len())
}

func TestWeightedQueueItemsEvictedFromHead(t *testing.T) {
	q := NewWeightedQueue(3, math.MaxInt64)
	exit := make(chan struct{})

	q.Add(newItem("item", 1))
	q.Add(newItem("item", 2))
	q.Add(newItem("item", 3))
	q.Add(newItem("item-new", 4))

	item, ok := q.Poll(exit)
	assert.True(t, ok)
	assert.Equal(t, "item", item.Type())
	assert.Equal(t, int64(2), item.Weight())

	item, ok = q.Poll(exit)
	assert.True(t, ok)
	assert.Equal(t, "item", item.Type())
	assert.Equal(t, int64(3), item.Weight())

	item, ok = q.Poll(exit)
	assert.True(t, ok)
	assert.Equal(t, "item-new", item.Type())
	assert.Equal(t, int64(4), item.Weight())

	assert.Equal(t, 0, q.Len())
	assert.Equal(t, int64(0), q.Weight())
}

func TestWeightedQueueItemsEvictedByTypeForWeight(t *testing.T) {
	q := NewWeightedQueue(100, 10)
	exit := make(chan struct{})

	q.Add(newItem("item1", 1))
	q.Add(newItem("item2", 7))
	q.Add(newItem("item1", 2))
	q.Add(newItem("item2", 4))

	item, ok := q.Poll(exit)
	assert.True(t, ok)
	assert.Equal(t, "item1", item.Type())
	assert.Equal(t, int64(1), item.Weight())

	item, ok = q.Poll(exit)
	assert.True(t, ok)
	assert.Equal(t, "item1", item.Type())
	assert.Equal(t, int64(2), item.Weight())

	item, ok = q.Poll(exit)
	assert.True(t, ok)
	assert.Equal(t, "item2", item.Type())
	assert.Equal(t, int64(4), item.Weight())

	assert.Equal(t, 0, q.Len())
}

func TestWeightedQueueItemsEvictedForWeight(t *testing.T) {
	q := NewWeightedQueue(100, 10)
	exit := make(chan struct{})

	q.Add(newItem("item1", 1))
	q.Add(newItem("item2", 7))
	q.Add(newItem("item1", 2))
	q.Add(newItem("item2", 10))

	item, ok := q.Poll(exit)
	assert.True(t, ok)
	assert.Equal(t, "item2", item.Type())
	assert.Equal(t, int64(10), item.Weight())

	assert.Equal(t, 0, q.Len())
	assert.Equal(t, int64(0), q.Weight())
}

func TestWeightedQueueAvailableWeightCorrectlySetEvictingItemsOfSameType(t *testing.T) {
	q := NewWeightedQueue(100, 10)
	exit := make(chan struct{})

	q.Add(newItem("item1", 1))
	q.Add(newItem("item1", 2))
	q.Add(newItem("item1", 3))
	q.Add(newItem("item1", 6))
	q.Add(newItem("item1", 4))

	item, ok := q.Poll(exit)
	assert.True(t, ok)
	assert.Equal(t, "item1", item.Type())
	assert.Equal(t, int64(6), item.Weight())

	item, ok = q.Poll(exit)
	assert.True(t, ok)
	assert.Equal(t, "item1", item.Type())
	assert.Equal(t, int64(4), item.Weight())

	assert.Equal(t, 0, q.Len())
}

func TestWeightedQueueAvailableWeightCorrectlySetEvictingItemsOfDifferentType(t *testing.T) {
	q := NewWeightedQueue(100, 10)
	exit := make(chan struct{})

	q.Add(newItem("item1", 1))
	q.Add(newItem("item1", 2))
	q.Add(newItem("item1", 3))
	q.Add(newItem("item2", 6))
	q.Add(newItem("item3", 4))

	item, ok := q.Poll(exit)
	assert.True(t, ok)
	assert.Equal(t, "item2", item.Type())
	assert.Equal(t, int64(6), item.Weight())

	item, ok = q.Poll(exit)
	assert.True(t, ok)
	assert.Equal(t, "item3", item.Type())
	assert.Equal(t, int64(4), item.Weight())

	assert.Equal(t, 0, q.Len())
}

func TestWeightedQueueAvailableWeightDecreasedAfterPoll(t *testing.T) {
	q := NewWeightedQueue(100, 10)
	exit := make(chan struct{})

	q.Add(newItem("item", 2))
	q.Add(newItem("item", 3))
	q.Add(newItem("item", 5))

	item, ok := q.Poll(exit)
	assert.True(t, ok)
	assert.Equal(t, "item", item.Type())
	assert.Equal(t, int64(2), item.Weight())

	item, ok = q.Poll(exit)
	assert.True(t, ok)
	assert.Equal(t, "item", item.Type())
	assert.Equal(t, int64(3), item.Weight())

	q.Add(newItem("item", 4))

	item, ok = q.Poll(exit)
	assert.True(t, ok)
	assert.Equal(t, "item", item.Type())
	assert.Equal(t, int64(5), item.Weight())

	item, ok = q.Poll(exit)
	assert.True(t, ok)
	assert.Equal(t, "item", item.Type())
	assert.Equal(t, int64(4), item.Weight())

	assert.Equal(t, 0, q.Len())
}

func newItem(name string, weight int64) WeightedItem {
	return &testItem{name: name, weight: weight}
}

type testItem struct {
	name   string
	weight int64
}

func (t *testItem) Weight() int64 {
	return t.weight
}

func (t *testItem) Type() string {
	return t.name
}

var _ WeightedItem = &testItem{}
