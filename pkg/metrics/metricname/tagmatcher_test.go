// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metricname

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTagNameExtraction(t *testing.T) {
	t.Run("extracts name from tag with value", func(t *testing.T) {
		assert.Equal(t, "env", tagName("env:prod"))
		assert.Equal(t, "host", tagName("host:server1"))
		assert.Equal(t, "version", tagName("version:1.0.0"))
	})

	t.Run("handles tag without value", func(t *testing.T) {
		assert.Equal(t, "env", tagName("env"))
		assert.Equal(t, "host", tagName("host"))
	})

	t.Run("handles tag with empty value", func(t *testing.T) {
		assert.Equal(t, "env", tagName("env:"))
	})

	t.Run("handles tag with colon in value", func(t *testing.T) {
		assert.Equal(t, "url", tagName("url:http://example.com"))
	})

	t.Run("handles invalid tag", func(t *testing.T) {
		assert.Equal(t, "", tagName(":invalid"))
	})
}

func TestNewHashedMetricTagListTagsAreSorted(t *testing.T) {
	tags := []string{"zzz", "aaa", "mmm", "bbb", "yyy"}

	assert.True(t, slices.IsSorted(NewHashedMetricTagList(true, tags).tags))
	assert.True(t, slices.IsSorted(NewHashedMetricTagList(false, tags).tags))
}

func TestKeepTag(t *testing.T) {
	exclude := NewHashedMetricTagList(true, []string{"env", "host"})
	assert.False(t, exclude.KeepTag("env:prod"))
	assert.False(t, exclude.KeepTag("host"))
	assert.True(t, exclude.KeepTag("version:1.0"))

	include := NewHashedMetricTagList(false, []string{"env", "host"})
	assert.True(t, include.KeepTag("env:prod"))
	assert.True(t, include.KeepTag("host"))
	assert.False(t, include.KeepTag("version:1.0"))
}

func TestTagMatcherZeroValue(t *testing.T) {
	var m TagMatcher
	_, ok := m.ShouldStripTags("metric")
	assert.False(t, ok)
	assert.Equal(t, 0, m.Len())
}

func TestTagMatcherDoesNotAllocate(t *testing.T) {
	m := NewTagMatcher(map[string]HashedMetricTagList{
		"metric": NewHashedMetricTagList(true, []string{"env", "host"}),
	})

	allocs := testing.AllocsPerRun(100, func() {
		tl, ok := m.ShouldStripTags("metric")
		if !ok || tl.KeepTag("env:prod") {
			t.Fatal("unexpected match result")
		}
	})
	assert.Zero(t, allocs)
}
