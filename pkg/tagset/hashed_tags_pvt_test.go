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
	assert.Equal(t, 0, len(ht.data))
	assert.Equal(t, 0, len(ht.hash))
}

func TestPvtNewHashedTagsFromSlice(t *testing.T) {
	ht := newHashedTagsFromSlice([]string{"abc", "def"})
	assert.Equal(t, []string{"abc", "def"}, ht.data)
	assert.Equal(t, []uint64{murmur3.StringSum64("abc"), murmur3.StringSum64("def")}, ht.hash)
}

func TestPvtHashedTagsCopy(t *testing.T) {
	ht := newHashedTagsFromSlice([]string{"abc", "def"})
	tags := ht.Copy()
	assert.Equal(t, ht.data, tags)
	// check that this is a copy
	tags[0] = "XXX"
	assert.NotEqual(t, "XXX", ht.data[0])
}

func TestPvtHashedTagsLen(t *testing.T) {
	ht := newHashedTagsFromSlice([]string{"abc", "def"})
	assert.Equal(t, 2, ht.Len())
}

func TestPvtHashedTagsDup(t *testing.T) {
	ht := newHashedTagsFromSlice([]string{"abc", "def"})
	ht2 := ht.dup()
	assert.Equal(t, ht.data, ht2.data)
	assert.Equal(t, ht.hash, ht2.hash)
	// check that this is a copy
	ht2.data[0] = "XXX"
	ht2.hash[0] = 999
	assert.NotEqual(t, "XXX", ht.data[0])
	assert.NotEqual(t, uint64(999), ht.hash[0])
}
