// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filterlistimpl

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zeebo/xxh3"
)

func TestNewTagMatcher(t *testing.T) {
	matcher := newTagMatcher(map[string]MetricTagList{
		"metric1": {
			Tags:   []string{"env", "host"},
			Action: "exclude",
		},
		"metric2": {
			Tags:   []string{},
			Action: "include",
		},
		"metric3": {
			Tags:   []string{"pod"},
			Action: "invalid",
		},
	})

	metric1Tags := []uint64{xxh3.HashString("env"), xxh3.HashString("host")}
	slices.Sort(metric1Tags)

	assert.NotNil(t, matcher)
	assert.Equal(t, matcher.MetricTags["metric1"], hashedMetricTagList{
		tags:   metric1Tags,
		action: Exclude,
	})

	assert.Equal(t, matcher.MetricTags["metric2"], hashedMetricTagList{
		tags:   []uint64{},
		action: Include,
	})

	assert.Equal(t, matcher.MetricTags["metric3"], hashedMetricTagList{
		tags:   []uint64{xxh3.HashString("pod")},
		action: Exclude,
	})
}

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

func TestTagMatcher(t *testing.T) {
	metrics := map[string]MetricTagList{
		"metric1": {
			Tags:   []string{"env", "host"},
			Action: "exclude",
		},
		"metric2": {
			Tags:   []string{"env", "host"},
			Action: "include",
		},
		"metric3": {
			Tags:   []string{},
			Action: "include",
		},
		"metric4": {
			Tags:   []string{},
			Action: "exclude",
		},
	}

	matcher := newTagMatcher(metrics)

	// Test metric1 tags are excluded
	keepTagFunc, shouldStrip := matcher.ShouldStripTags("metric1")
	assert.True(t, shouldStrip)

	assert.False(t, keepTagFunc("env:prod"))
	assert.False(t, keepTagFunc("host:server1"))
	assert.True(t, keepTagFunc("version:1.0"))

	// Test metric2 tags are included
	keepTagFunc, shouldStrip = matcher.ShouldStripTags("metric2")
	assert.True(t, shouldStrip)

	assert.True(t, keepTagFunc("env:prod"))
	assert.True(t, keepTagFunc("host:server1"))
	assert.False(t, keepTagFunc("version:1.0"))

	// Test metric3 tags are all excluded
	keepTagFunc, shouldStrip = matcher.ShouldStripTags("metric3")
	assert.True(t, shouldStrip)

	assert.False(t, keepTagFunc("env:prod"))
	assert.False(t, keepTagFunc("host:server1"))
	assert.False(t, keepTagFunc("version:1.0"))

	// Test metric4 tags are all included
	keepTagFunc, shouldStrip = matcher.ShouldStripTags("metric4")
	if shouldStrip { // 2 behaviors are acceptable: return true with a function that keeps all tags, or return false
		assert.True(t, keepTagFunc("env:prod"))
		assert.True(t, keepTagFunc("host:server1"))
		assert.True(t, keepTagFunc("version:1.0"))
	}

	// metric5 is not configured
	_, shouldStrip = matcher.ShouldStripTags("metric5")
	assert.False(t, shouldStrip)
}

func TestShouldStripTagsByNameHash(t *testing.T) {
	metrics := map[string]MetricTagList{
		"metric1": {
			Tags:   []string{"env", "host"},
			Action: "exclude",
		},
		"metric2": {
			Tags:   []string{"env", "host"},
			Action: "include",
		},
		"metric3": {
			Tags:   []string{},
			Action: "include",
		},
	}

	matcher := newTagMatcher(metrics)

	hashOf := func(tag string) uint64 {
		return xxh3.HashString(tagName(tag))
	}

	// metric1: exclude "env" and "host" — those hashes should return false (strip them)
	keepByHash, ok := matcher.ShouldStripTagsByNameHash("metric1")
	assert.True(t, ok)
	assert.False(t, keepByHash(hashOf("env:prod")))
	assert.False(t, keepByHash(hashOf("host:server1")))
	assert.True(t, keepByHash(hashOf("version:1.0")))

	// metric2: include only "env" and "host" — only those hashes should return true
	keepByHash, ok = matcher.ShouldStripTagsByNameHash("metric2")
	assert.True(t, ok)
	assert.True(t, keepByHash(hashOf("env:prod")))
	assert.True(t, keepByHash(hashOf("host:server1")))
	assert.False(t, keepByHash(hashOf("version:1.0")))

	// metric3: include with empty tag list — all tags are excluded
	keepByHash, ok = matcher.ShouldStripTagsByNameHash("metric3")
	assert.True(t, ok)
	assert.False(t, keepByHash(hashOf("env:prod")))
	assert.False(t, keepByHash(hashOf("version:1.0")))

	// unconfigured metric
	keepByHash, ok = matcher.ShouldStripTagsByNameHash("metric_unknown")
	assert.False(t, ok)
	assert.Nil(t, keepByHash)
}

// TestShouldStripTagsByNameHashMatchesStringFilter verifies that the hash-based filter
// returns identical results to the string-based filter for every tag.
func TestShouldStripTagsByNameHashMatchesStringFilter(t *testing.T) {
	tags := []string{"env:prod", "host:server1", "version:1.0", "region:us-east", "service:web"}

	for _, tc := range []struct {
		name   string
		action string
	}{
		{"exclude-metric", "exclude"},
		{"include-metric", "include"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			matcher := newTagMatcher(map[string]MetricTagList{
				tc.name: {Tags: []string{"env", "host"}, Action: tc.action},
			})

			keepStr, _ := matcher.ShouldStripTags(tc.name)
			keepHash, _ := matcher.ShouldStripTagsByNameHash(tc.name)

			for _, tag := range tags {
				wantStr := keepStr(tag)
				gotHash := keepHash(xxh3.HashString(tagName(tag)))
				assert.Equal(t, wantStr, gotHash, "mismatch for tag %q", tag)
			}
		})
	}
}
