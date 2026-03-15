// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/twmb/murmur3"
)

func TestPvtNewHashedTagsWithCapacity(t *testing.T) {
	ht := newHashedTagsWithCapacity(10)
	assert.Equal(t, 0, len(ht.tags))
}

func TestPvtNewHashedTagsFromSlice(t *testing.T) {
	ht := newHashedTagsFromSlice([]string{"abc", "def"})
	assert.Equal(t, []TagHash{
		{Tag: "abc", Hash: murmur3.StringSum64("abc")},
		{Tag: "def", Hash: murmur3.StringSum64("def")},
	}, ht.tags)
}

func TestPvtHashedTagsCopy(t *testing.T) {
	ht := newHashedTagsFromSlice([]string{"abc", "def"})
	tags := ht.Copy()
	assert.Equal(t, []string{"abc", "def"}, tags)
	// check that this is a copy
	tags[0] = "XXX"
	assert.NotEqual(t, "XXX", ht.tags[0].Tag)
}

func TestPvtHashedTagsLen(t *testing.T) {
	ht := newHashedTagsFromSlice([]string{"abc", "def"})
	assert.Equal(t, 2, ht.Len())
}

func TestPvtHashedTagsDup(t *testing.T) {
	ht := newHashedTagsFromSlice([]string{"abc", "def"})
	ht2 := ht.dup()
	assert.Equal(t, ht.tags, ht2.tags)
	// check that this is a copy
	ht2.tags[0] = TagHash{Tag: "XXX", Hash: 999}
	assert.NotEqual(t, "XXX", ht.tags[0].Tag)
	assert.NotEqual(t, uint64(999), ht.tags[0].Hash)
}
