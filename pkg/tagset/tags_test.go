// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/twmb/murmur3"
)

var fooHash, barHash uint64

func init() {
	fooHash = murmur3.StringSum64("foo")
	barHash = murmur3.StringSum64("bar")
}

func newTags(tags []string) *Tags {
	hashes := make([]uint64, len(tags))
	hash := uint64(0)
	for i, t := range tags {
		h := murmur3.StringSum64(t)
		hashes[i] = h
		hash ^= h
	}
	return &Tags{tags, hashes, hash}
}

// validate is a testing-only function that will validate that the
// given Tags instance is well-formed (has correct hashes)
func (tags *Tags) validate(t *testing.T) {
	require.Equal(t, len(tags.tags), len(tags.hashes))
	hash := uint64(0)
	seenT := make(map[string]struct{})
	seenH := make(map[uint64]struct{})
	for i, tag := range tags.tags {
		if _, s := seenT[tag]; s {
			require.Fail(t, "not unique",
				"tag %#v appears multiple times", tag)
		}
		seenT[tag] = struct{}{}

		h := murmur3.StringSum64(tag)
		if _, s := seenH[h]; s {
			require.Fail(t, "not unique",
				"hash 0x%016x appears multiple times", h)
		}
		seenH[h] = struct{}{}

		require.Equal(t, h, tags.hashes[i],
			"hash at index %d should be 0x%016x, got 0x%016x", i, h, tags.hashes[i])
		hash ^= h
	}
	require.Equal(t, hash, tags.hash, "Tags.hash is incorrect")
}

func TestTags_Tags_String(t *testing.T) {
	tagset := newTags([]string{"foo", "bar"})
	require.Equal(t, tagset.String(), "foo, bar")
}

func TestTags_Tags_MarshalJSON(t *testing.T) {
	tagset := newTags([]string{"foo", "bar"})
	j, err := tagset.MarshalJSON()
	require.NoError(t, err)
	require.Equal(t, []byte(`["foo","bar"]`), j)
}

func TestTags_Tags_MarshalJSON_empty(t *testing.T) {
	j, err := json.Marshal(EmptyTags)
	require.NoError(t, err)
	require.Equal(t, []byte(`[]`), j)
}

func TestTags_Tags_Hash(t *testing.T) {
	tagset := newTags([]string{"foo"})
	require.Equal(t, tagset.Hash(), fooHash)
}

func TestTags_Tags_Sorted(t *testing.T) {
	tagset := newTags([]string{"foo", "bar"})
	require.Equal(t, tagset.Sorted(), []string{"bar", "foo"})
}

func TestTags_Contains(t *testing.T) {
	tagset := newTags([]string{"foo", "bar"})
	t.Run("foo", func(t *testing.T) {
		require.True(t, tagset.Contains("foo"))
	})
	t.Run("baz", func(t *testing.T) {
		require.False(t, tagset.Contains("baz"))
	})
}

func TestTags_IsSubsetOf(t *testing.T) {
	ts1 := newTags([]string{"foo", "bar", "bing"})
	ts2 := newTags([]string{"bar", "bing"})
	ts3 := newTags([]string{"bar", "bing", "baz"})

	test := func(name string, a, b *Tags, isSubset bool) {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, a.IsSubsetOf(b), isSubset)
		})
	}

	test("ts1 ts1", ts1, ts1, true)
	test("ts1 ts2", ts1, ts2, false)
	test("ts1 ts3", ts1, ts3, false)
	test("ts2 ts1", ts2, ts1, true)
	test("ts2 ts2", ts2, ts2, true)
	test("ts2 ts3", ts2, ts3, true)
	test("ts3 ts1", ts3, ts1, false)
	test("ts3 ts2", ts3, ts2, false)
	test("ts3 ts3", ts3, ts3, true)
}

func TestTags_WithKey(t *testing.T) {
	tagset := newTags([]string{"foo:1", "bar:2", "foo:2"})
	fooTags := tagset.WithKey("foo")
	sort.Strings(fooTags)
	require.Equal(t, fooTags, []string{"foo:1", "foo:2"})
	require.Equal(t, tagset.WithKey("bar"), []string{"bar:2"})
}

func TestTags_FindByKey(t *testing.T) {
	tagset := newTags([]string{"foo:1", "bar:2", "foo:2"})
	require.Contains(t, []string{"foo:1", "foo:2"}, tagset.FindByKey("foo"))
	require.Equal(t, tagset.FindByKey("bar"), "bar:2")
}

func TestTags_Range(t *testing.T) {
	tagset := newTags([]string{"foo:1", "bar:2", "foo:2"})
	got := []string{}
	tagset.ForEach(func(t string) { got = append(got, t) })
	sort.Strings(got)
	require.Equal(t, got, []string{"bar:2", "foo:1", "foo:2"})
}

func TestTags_ReadOnlySlice(t *testing.T) {
	tagset := newTags([]string{"foo", "bar"})
	slice := tagset.UnsafeReadOnlySlice()
	require.Equal(t, len(slice), 2)
	require.Contains(t, slice, "foo")
	require.Contains(t, slice, "bar")
}
