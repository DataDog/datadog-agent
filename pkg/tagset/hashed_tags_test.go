// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHashedTagsSlice(t *testing.T) {
	ht := NewHashedTagsFromSlice([]string{"a", "b", "c", "d", "e"})
	ht2 := ht.Slice(1, 3)
	assert.Equal(t, ht2.Get(), []string{"b", "c"})
	assert.Equal(t, ht2.Hashes(), ht.Hashes()[1:3])
}

func TestHashedTagsGet(t *testing.T) {
	ht := NewHashedTagsFromSlice([]string{"abc", "def"})
	tags := ht.Get()
	assert.Equal(t, ht.data, tags)
	// check that this is *not* a copy
	tags[0] = "XXX"
	assert.Equal(t, "XXX", ht.data[0])
}

func TestHashedTagsHashes(t *testing.T) {
	ht := NewHashedTagsFromSlice([]string{"abc", "def"})
	hashes := ht.Hashes()
	log.Printf("%p %p", ht.hash, hashes)
	assert.Equal(t, ht.hash, hashes)
	// check that this is *not* a copy
	hashes[0] = 999
	assert.Equal(t, uint64(999), ht.hash[0])
}
