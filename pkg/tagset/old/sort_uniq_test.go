// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInsertionSort(t *testing.T) {
	assert := assert.New(t)

	tags := []string{
		"zzz",
		"hello:world",
		"world:hello",
		"random2:value",
		"random1:value",
	}

	InsertionSort(tags)

	assert.Equal("hello:world", tags[0])
	assert.Equal("random1:value", tags[1])
	assert.Equal("random2:value", tags[2])
	assert.Equal("world:hello", tags[3])
	assert.Equal("zzz", tags[4])
}
func TestSortUniqInPlace(t *testing.T) {
	elements := []string{"tag3:tagggg", "tag2:tagval", "tag1:tagval", "tag2:tagval"}
	elements = SortUniqInPlace(elements)

	assert.ElementsMatch(t, elements, []string{"tag1:tagval", "tag2:tagval", "tag3:tagggg"})
}

func benchmarkDeduplicateTags(b *testing.B, numberOfTags int) {
	tags := make([]string, 0, numberOfTags+1)
	for i := 0; i < numberOfTags; i++ {
		tags = append(tags, fmt.Sprintf("aveeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeerylong:tag%d", i))
	}
	// this is the worst case for the insertion sort we are using
	sort.Sort(sort.Reverse(sort.StringSlice(tags)))

	tempTags := make([]string, len(tags))
	copy(tempTags, tags)
	b.ReportAllocs()
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		copy(tempTags, tags)
		SortUniqInPlace(tempTags)
	}
}
func BenchmarkDeduplicateTags(b *testing.B) {
	for i := 1; i <= 128; i *= 2 {
		b.Run(fmt.Sprintf("deduplicate-%d-tags-in-place", i), func(b *testing.B) {
			benchmarkDeduplicateTags(b, i)
		})
	}
}
