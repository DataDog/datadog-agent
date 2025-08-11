// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"iter"
	"testing"

	"github.com/stretchr/testify/require"
)

func (q *queue[Item, ID]) items() iter.Seq[Item] {
	return q.list.items()
}

// Note that it's not safe to modify the list while iterating over it.
func (l *list[T]) items() iter.Seq[T] {
	return func(yield func(T) bool) {
		if l.head == nil {
			return
		}
		cur := l.head
		for {
			if !yield(cur.value) {
				return
			}
			if cur = cur.next; cur == l.head {
				return
			}
		}
	}
}

func TestQueue(t *testing.T) {
	type testItem struct {
		ID   int
		Name string
	}
	testItemID := func(i testItem) int { return i.ID }

	t.Run("new queue is empty", func(t *testing.T) {
		q := makeQueue(testItemID)
		_, ok := q.popFront()
		require.False(t, ok)
		require.Equal(t, 0, len(q.m))
		require.Nil(t, q.list.head)
	})

	t.Run("push and pop", func(t *testing.T) {
		q := makeQueue(testItemID)
		item1 := testItem{ID: 1, Name: "one"}
		_, hadPrev := q.pushBack(item1)
		require.False(t, hadPrev)

		require.Equal(t, 1, len(q.m))
		_, ok := q.m[1]
		require.True(t, ok)

		item, ok := q.popFront()
		require.True(t, ok)
		require.Equal(t, item1, item)
		require.Equal(t, 0, len(q.m))

		_, ok = q.popFront()
		require.False(t, ok)
	})

	t.Run("FIFO order", func(t *testing.T) {
		q := makeQueue(testItemID)
		items := []testItem{
			{ID: 1, Name: "one"},
			{ID: 2, Name: "two"},
			{ID: 3, Name: "three"},
		}

		for _, item := range items {
			_, hadPrev := q.pushBack(item)
			require.False(t, hadPrev)
		}
		require.Equal(t, len(items), len(q.m))

		for i := 0; i < len(items); i++ {
			item, ok := q.popFront()
			require.True(t, ok)
			require.Equal(t, items[i], item)
			require.Equal(t, len(items)-i-1, len(q.m))
		}

		_, ok := q.popFront()
		require.False(t, ok)
		require.Equal(t, 0, len(q.m))
	})

	t.Run("remove", func(t *testing.T) {
		type testCase struct {
			name        string
			items       []testItem
			removeID    int
			expected    []testItem
			removedItem testItem
			removeOk    bool
		}

		testCases := []testCase{
			{
				name:        "from middle",
				items:       []testItem{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}, {ID: 3, Name: "c"}},
				removeID:    2,
				expected:    []testItem{{ID: 1, Name: "a"}, {ID: 3, Name: "c"}},
				removedItem: testItem{ID: 2, Name: "b"},
				removeOk:    true,
			},
			{
				name:        "head",
				items:       []testItem{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}, {ID: 3, Name: "c"}},
				removeID:    1,
				expected:    []testItem{{ID: 2, Name: "b"}, {ID: 3, Name: "c"}},
				removedItem: testItem{ID: 1, Name: "a"},
				removeOk:    true,
			},
			{
				name:        "tail",
				items:       []testItem{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}, {ID: 3, Name: "c"}},
				removeID:    3,
				expected:    []testItem{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}},
				removedItem: testItem{ID: 3, Name: "c"},
				removeOk:    true,
			},
			{
				name:        "the only item",
				items:       []testItem{{ID: 1, Name: "a"}},
				removeID:    1,
				expected:    []testItem{},
				removedItem: testItem{ID: 1, Name: "a"},
				removeOk:    true,
			},
			{
				name:        "non-existent",
				items:       []testItem{{ID: 1, Name: "a"}},
				removeID:    2,
				expected:    []testItem{{ID: 1, Name: "a"}},
				removedItem: testItem{},
				removeOk:    false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				q := makeQueue(testItemID)
				for _, item := range tc.items {
					q.pushBack(item)
				}

				removed, ok := q.remove(tc.removeID)
				require.Equal(t, tc.removeOk, ok)
				if ok {
					require.Equal(t, tc.removedItem, removed)
				}

				got := make([]testItem, 0) // not nil because equality is checked
				for {
					item, ok := q.popFront()
					if !ok {
						break
					}
					got = append(got, item)
				}
				require.Equal(t, tc.expected, got)
				require.Zero(t, len(q.m))
			})
		}
	})

	t.Run("interleaved operations", func(t *testing.T) {
		q := makeQueue(testItemID)

		q.pushBack(testItem{ID: 1})
		q.pushBack(testItem{ID: 2})

		item, ok := q.popFront()
		require.True(t, ok)
		require.Equal(t, 1, item.ID)
		require.Equal(t, 1, len(q.m))

		q.pushBack(testItem{ID: 3})
		require.Equal(t, 2, len(q.m))

		removed, ok := q.remove(2)
		require.True(t, ok)
		require.Equal(t, 2, removed.ID)
		require.Equal(t, 1, len(q.m))

		q.pushBack(testItem{ID: 4})
		require.Equal(t, 2, len(q.m))

		item, ok = q.popFront()
		require.True(t, ok)
		require.Equal(t, 3, item.ID)
		require.Equal(t, 1, len(q.m))

		item, ok = q.popFront()
		require.True(t, ok)
		require.Equal(t, 4, item.ID)
		require.Equal(t, 0, len(q.m))

		_, ok = q.popFront()
		require.False(t, ok)
	})

	t.Run("push with duplicate ID", func(t *testing.T) {
		q := makeQueue(testItemID)
		q.pushBack(testItem{ID: 1, Name: "first"})
		q.pushBack(testItem{ID: 1, Name: "second"})

		// The current implementation has inconsistent state when pushing an item with
		// an ID that already exists. The map entry is overwritten, but the old item
		// remains in the list, orphaned.
		// A push with a duplicate ID should replace the existing item (upsert).
		// This test expects the upsert behavior and will fail on the current implementation.
		require.Equal(t, 1, len(q.m), "map should have one entry")

		item, ok := q.popFront()
		require.True(t, ok)
		require.Equal(t, testItem{ID: 1, Name: "second"}, item, "should get the last item pushed with the ID")

		_, ok = q.popFront()
		require.False(t, ok, "queue should be empty")
	})
}
