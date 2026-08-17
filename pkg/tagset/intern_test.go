// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"fmt"
	"sync"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/murmur3"
)

// unsafeStringData identifies the backing array of a string, to show that two
// interned references share one copy rather than each holding their own.
func unsafeStringData(s string) *byte {
	return unsafe.StringData(s)
}

func TestInternedTagValueAndHash(t *testing.T) {
	tag := Intern("env:prod")
	assert.Equal(t, "env:prod", tag.Value())
	assert.Equal(t, murmur3.StringSum64("env:prod"), tag.Hash())

	// the nil tag is usable, so a zero-valued sample does not panic
	var empty InternedTag
	assert.Equal(t, "", empty.Value())
	assert.Equal(t, murmur3.StringSum64(""), empty.Hash())
}

func TestTableLoadOrStoreDeduplicates(t *testing.T) {
	table := NewTable(4)

	first, found := table.LoadOrStore([]byte("env:prod"))
	assert.False(t, found)
	second, found := table.LoadOrStore([]byte("env:prod"))
	assert.True(t, found)

	assert.Same(t, first, second, "the same tag must resolve to the same interned copy")
	assert.Equal(t, 1, table.Len())

	// and the strings share storage rather than being separate copies
	assert.Equal(t, unsafeStringData(first.Value()), unsafeStringData(second.Value()))
}

func TestTableLoadOrStoreStringMatchesBytes(t *testing.T) {
	table := NewTable(4)

	fromBytes, _ := table.LoadOrStore([]byte("env:prod"))
	fromString, found := table.LoadOrStoreString("env:prod")

	assert.True(t, found)
	assert.Same(t, fromBytes, fromString)
}

func TestTableGrowsWithoutACap(t *testing.T) {
	table := NewTable(4)

	for i := 0; i < 10_000; i++ {
		table.LoadOrStore([]byte(fmt.Sprintf("tag:%d", i)))
	}

	assert.Equal(t, 10_000, table.Len(),
		"the table is sized by liveness, so tags still arriving are never dropped")
}

func TestTableEvictsTagsThatStopArriving(t *testing.T) {
	table := NewTable(4)

	var evicted int
	table.SetEvictionCallback(func(n int) { evicted += n })

	stale, _ := table.LoadOrStore([]byte("stale:tag"))
	fresh, _ := table.LoadOrStore([]byte("fresh:tag"))
	require.Equal(t, 2, table.Len())

	// advance past the retention window, keeping only one of the two alive
	for i := 0; i <= tableEpochsRetained+1; i++ {
		table.advanceEpoch()
		table.LoadOrStoreString("fresh:tag")
	}

	assert.Equal(t, 1, table.Len(), "the tag that stopped arriving must be evicted")
	assert.Equal(t, 1, evicted)

	_, found := table.LoadOrStoreString("fresh:tag")
	assert.True(t, found, "the tag that kept arriving must be retained")

	// An evicted tag stays valid for whoever still holds it: samples and contexts
	// keep the *Tag directly, so eviction can never invalidate them.
	assert.Equal(t, "stale:tag", stale.Value())
	assert.Equal(t, murmur3.StringSum64("stale:tag"), stale.Hash())
	assert.Equal(t, "fresh:tag", fresh.Value())
}

func TestTableReinternsAfterEviction(t *testing.T) {
	table := NewTable(4)

	before, _ := table.LoadOrStore([]byte("quiet:tag"))
	for i := 0; i <= tableEpochsRetained+1; i++ {
		table.advanceEpoch()
	}
	require.Equal(t, 0, table.Len())

	after, found := table.LoadOrStore([]byte("quiet:tag"))
	assert.False(t, found, "an evicted tag is interned afresh when it comes back")
	assert.NotSame(t, before, after)

	// The transient duplicate is still a correct tag: same value, same hash, so it
	// dedupes against the old copy anywhere tag identity is (hash, value).
	assert.Equal(t, before.Value(), after.Value())
	assert.Equal(t, before.Hash(), after.Hash())
}

func TestAppendInternedMatchesAppend(t *testing.T) {
	table := NewTable(4)
	tags := []InternedTag{}
	for _, s := range []string{"env:prod", "service:api", "az:us-east-1a"} {
		tag, _ := table.LoadOrStoreString(s)
		tags = append(tags, tag)
	}

	interned := NewHashingTagsAccumulator()
	interned.AppendInterned(tags...)

	plain := NewHashingTagsAccumulatorWithTags([]string{"env:prod", "service:api", "az:us-east-1a"})

	assert.Equal(t, plain.Get(), interned.Get())
	assert.Equal(t, plain.Hashes(), interned.Hashes(),
		"memoized hashes must match what Append computes, or context keys would diverge")
	assert.Equal(t, plain.Hash(), interned.Hash())
}

func TestValues(t *testing.T) {
	assert.Nil(t, Values(nil))
	assert.Equal(t, []string{"a", "b"}, Values(InternAll([]string{"a", "b"})))
}

// TestTableConcurrentLoadOrStore is the reason the table is locked: every
// dogstatsd worker shares one, so lookups, interning and sweeps all race.
// Run with -race.
func TestTableConcurrentLoadOrStore(t *testing.T) {
	const (
		workers    = 8
		iterations = 2000
		distinct   = 64
	)
	table := NewTable(8)

	var wg sync.WaitGroup
	seen := make([][]InternedTag, workers)
	for w := 0; w < workers; w++ {
		seen[w] = make([]InternedTag, distinct)
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				key := []byte(fmt.Sprintf("tag:%d", i%distinct))
				tag, _ := table.LoadOrStore(key)
				seen[w][i%distinct] = tag
			}
		}(w)
	}
	// sweep concurrently with the lookups
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			table.advanceEpoch()
		}
	}()
	wg.Wait()

	// Every worker must have resolved each tag to the *same* interned copy: that
	// is what sharing the table buys over one table per worker.
	for i := 0; i < distinct; i++ {
		want := seen[0][i]
		require.NotNil(t, want)
		assert.Equal(t, fmt.Sprintf("tag:%d", i), want.Value())
		for w := 1; w < workers; w++ {
			assert.Same(t, want, seen[w][i], "workers must share one copy of each tag")
		}
	}
}

func TestTableConcurrentInternOfTheSameNewTag(t *testing.T) {
	// All workers race to intern the same previously-unseen tag; the double-checked
	// insert must hand every one of them the same tag, and count one entry.
	const workers = 16
	table := NewTable(1)

	var wg sync.WaitGroup
	got := make([]InternedTag, workers)
	start := make(chan struct{})
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			<-start
			got[w], _ = table.LoadOrStore([]byte("contended:tag"))
		}(w)
	}
	close(start)
	wg.Wait()

	assert.Equal(t, 1, table.Len())
	for w := 1; w < workers; w++ {
		assert.Same(t, got[0], got[w])
	}
}

func BenchmarkTableLoadOrStoreHit(b *testing.B) {
	table := NewTable(64)
	keys := make([][]byte, 32)
	for i := range keys {
		keys[i] = []byte(fmt.Sprintf("tag%d:value%d", i, i))
		table.LoadOrStore(keys[i])
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		table.LoadOrStore(keys[i%len(keys)])
	}
}

// BenchmarkTableLoadOrStoreHitParallel measures the cost that sharing the table
// introduces: all workers now take the same read lock on the hottest path in the
// agent, so RLock/RUnlock contend on one cache line.
func BenchmarkTableLoadOrStoreHitParallel(b *testing.B) {
	table := NewTable(64)
	keys := make([][]byte, 32)
	for i := range keys {
		keys[i] = []byte(fmt.Sprintf("tag%d:value%d", i, i))
		table.LoadOrStore(keys[i])
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			table.LoadOrStore(keys[i%len(keys)])
			i++
		}
	})
}
