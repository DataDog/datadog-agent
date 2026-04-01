// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/twmb/murmur3"
)

func TestNewHashingTagsAccumulator(t *testing.T) {
	tb := NewHashingTagsAccumulator()
	assert.NotNil(t, tb)
	assert.Equal(t, []string{}, tb.data)
}

func TestNewHashingTagsAccumulatorWithTags(t *testing.T) {
	test := []string{"a", "b", "c"}
	tb := NewHashingTagsAccumulatorWithTags(test)
	assert.NotNil(t, tb)
	assert.Equal(t, test, tb.data)
}

func TestHashingTagsAccumulatorAppend(t *testing.T) {
	tb := NewHashingTagsAccumulator()

	tb.Append("a", "b", "c")
	assert.Equal(t, []string{"a", "b", "c"}, tb.data)

	tb.Append("d")
	assert.Equal(t, []string{"a", "b", "c", "d"}, tb.data)
}

func TestHashingTagsAccumulatorReset(t *testing.T) {
	tb := NewHashingTagsAccumulator()

	tb.Append("a", "b", "c")
	assert.Equal(t, []string{"a", "b", "c"}, tb.data)

	tb.Reset()
	assert.Equal(t, []string{}, tb.data)
}

func TestHashingTagsAccumulatorGet(t *testing.T) {
	tb := NewHashingTagsAccumulator()

	tb.Append("a", "b", "c")
	internalData := tb.Get()
	assert.Equal(t, []string{"a", "b", "c"}, internalData)

	// check that the internal buffer was indeed returned and not a copy
	internalData[0] = "test"
	assert.Equal(t, []string{"test", "b", "c"}, internalData)
	assert.Equal(t, []string{"test", "b", "c"}, tb.data)
}

func TestHashingTagsAccumulatorCopy(t *testing.T) {
	tb := NewHashingTagsAccumulator()

	tb.Append("a", "b", "c")
	tagsCopy := tb.Copy()
	assert.Equal(t, []string{"a", "b", "c"}, tagsCopy)
	assert.NotSame(t, &tagsCopy, &tb.data)

	tagsCopy[0] = "test"
	assert.Equal(t, []string{"test", "b", "c"}, tagsCopy)
	assert.Equal(t, []string{"a", "b", "c"}, tb.data)
}

func TestRemoveSorted(t *testing.T) {
	l := NewHashingTagsAccumulator()
	r := NewHashingTagsAccumulator()
	l.Append("a", "b", "c", "d")
	l.SortUniq()
	r.Append("a", "b", "e", "f")
	r.SortUniq()
	r.removeSorted(l)
	assert.ElementsMatch(t, []string{"a", "b", "c", "d"}, l.Get())
	assert.ElementsMatch(t, []string{"e", "f"}, r.Get())

	r.Reset()
	r.Append("c", "d", "e", "f")
	r.SortUniq()
	r.removeSorted(l)
	assert.ElementsMatch(t, []string{"e", "f"}, r.Get())

	r.Reset()
	r.Append("a", "aa", "ab", "b")
	r.SortUniq()
	r.removeSorted(l)
	assert.ElementsMatch(t, []string{"aa", "ab"}, r.Get())

	r.Reset()
	r.Append("A", "B", "a", "d")
	r.SortUniq()
	r.removeSorted(l)
	assert.ElementsMatch(t, []string{"A", "B"}, r.Get())

	r.Reset()
	r.Append("A", "a", "b", "e")
	r.SortUniq()
	r.removeSorted(l)
	assert.ElementsMatch(t, []string{"A", "e"}, r.Get())
}

func TestRemoveSortedHashCollision(t *testing.T) {
	const collisionHash = uint64(0xdeadbeef)

	// h's tag is alphabetically before the colliding tag in o — no match, tag must be kept.
	h := NewHashingTagsAccumulator()
	h.data = []string{"tag:keep"}
	h.hash = []uint64{collisionHash}

	o := NewHashingTagsAccumulator()
	o.data = []string{"tag:other"} // same hash, different string; "tag:other" > "tag:keep"
	o.hash = []uint64{collisionHash}

	done := make(chan struct{})
	go func() {
		h.removeSorted(o)
		close(done)
	}()

	select {
	case <-done:
		assert.ElementsMatch(t, []string{"tag:keep"}, h.Get())
	case <-time.After(3 * time.Second):
		t.Fatal("removeSorted hung: infinite loop on hash collision")
	}
}

func TestRemoveSortedHashCollisionWithMatch(t *testing.T) {
	const collisionHash = uint64(0xdeadbeef)

	// h's tag is alphabetically AFTER the colliding tag in o, but o also contains h's tag — it must be removed.
	// o sorted by (hash, string): ["tag:aaa"@collisionHash, "tag:zoo"@collisionHash]
	h := NewHashingTagsAccumulator()
	h.data = []string{"tag:zoo"}
	h.hash = []uint64{collisionHash}

	o := NewHashingTagsAccumulator()
	o.data = []string{"tag:aaa", "tag:zoo"} // "tag:aaa" collides with same hash, comes before "tag:zoo"
	o.hash = []uint64{collisionHash, collisionHash}

	h.removeSorted(o)
	assert.Empty(t, h.Get(), "tag:zoo must be removed since o contains it, despite hash collision with tag:aaa")
}

func testTagsMatchHash(t *testing.T, acc *HashingTagsAccumulator) {
	assert.Equal(t, len(acc.data), len(acc.hash))
	for idx, tag := range acc.data {
		assert.Equal(t, murmur3.StringSum64(tag), acc.hash[idx])
	}
}

func TestFilterTags(t *testing.T) {
	tests := []struct {
		name         string
		inputTags    []string
		keepFunc     func(string) bool
		expectedTags []string
	}{
		{
			name:         "filter all tags",
			inputTags:    []string{"env:prod", "host:server1", "version:1.0"},
			keepFunc:     func(_ string) bool { return false },
			expectedTags: []string{},
		},
		{
			name:         "keep all tags",
			inputTags:    []string{"env:prod", "host:server1", "version:1.0"},
			keepFunc:     func(_ string) bool { return true },
			expectedTags: []string{"env:prod", "host:server1", "version:1.0"},
		},
		{
			name:      "filter some tags",
			inputTags: []string{"env:prod", "host:server1", "version:1.0", "region:us-east"},
			keepFunc: func(tag string) bool {
				return tag == "env:prod" || tag == "version:1.0"
			},
			expectedTags: []string{"env:prod", "version:1.0"},
		},
		{
			name:         "no tags to filter",
			inputTags:    []string{},
			keepFunc:     func(_ string) bool { return true },
			expectedTags: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			acc := NewHashingTagsAccumulatorWithTags(tt.inputTags)
			removed := acc.RetainFunc(tt.keepFunc)

			assert.Equal(t, tt.expectedTags, acc.Get())
			assert.Equal(t, len(tt.inputTags)-len(tt.expectedTags), removed)
			testTagsMatchHash(t, acc)
		})
	}
}
