// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filterlistimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/twmb/murmur3"
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

	assert.NotNil(t, matcher)
	assert.Equal(t, matcher.MetricTags["metric1"], hashedMetricTagList{
		tags:   []uint64{murmur3.StringSum64("env"), murmur3.StringSum64("host")},
		action: Exclude,
	})

	assert.Equal(t, matcher.MetricTags["metric2"], hashedMetricTagList{
		tags:   []uint64{},
		action: Include,
	})

	assert.Equal(t, matcher.MetricTags["metric3"], hashedMetricTagList{
		tags:   []uint64{murmur3.StringSum64("pod")},
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
