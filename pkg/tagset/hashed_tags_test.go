// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zeebo/xxh3"
)

func tagNameForTest(tag string) string {
	pos := strings.IndexByte(tag, ':')
	if pos < 0 {
		return tag
	}
	return tag[:pos]
}

func TestHashedTagsSlice(t *testing.T) {
	ht := NewHashedTagsFromSlice([]string{"a", "b", "c", "d", "e"})
	ht2 := ht.Slice(1, 3)
	assert.Equal(t, ht2.Get(), []string{"b", "c"})
	assert.Equal(t, ht2.hash, ht.hash[1:3])
}

func TestHashedTagsGet(t *testing.T) {
	ht := NewHashedTagsFromSlice([]string{"abc", "def"})
	tags := ht.Get()
	assert.Equal(t, ht.data, tags)
	// check that this is *not* a copy
	tags[0] = "XXX"
	assert.Equal(t, "XXX", ht.data[0])
}

func TestHashedTagsNameHashPopulated(t *testing.T) {
	input := []string{"env:prod", "host:server1", "version:1.0", "novalue"}
	ht := NewHashedTagsFromSlice(input)

	assert.Equal(t, len(input), len(ht.nameHash), "nameHash length must match tag count")
	for i, tag := range input {
		want := xxh3.HashString(tagNameForTest(tag))
		assert.Equal(t, want, ht.nameHash[i], "nameHash[%d] mismatch for tag %q", i, tag)
	}
}

func TestHashedTagsSliceForwardsNameHash(t *testing.T) {
	ht := NewHashedTagsFromSlice([]string{"a:1", "b:2", "c:3", "d:4", "e:5"})
	sub := ht.Slice(1, 4)

	assert.Equal(t, ht.nameHash[1:4], sub.nameHash)
	assert.Equal(t, []string{"b:2", "c:3", "d:4"}, sub.Get())
}

func TestHashedTagsSliceNilNameHash(t *testing.T) {
	// A HashedTags built without nameHash (e.g. zero-value) should not panic on Slice.
	ht := HashedTags{
		hashedTags: newHashedTagsFromSlice([]string{"a", "b", "c"}),
	}
	sub := ht.Slice(0, 2)
	assert.Nil(t, sub.nameHash)
	assert.Equal(t, []string{"a", "b"}, sub.Get())
}
