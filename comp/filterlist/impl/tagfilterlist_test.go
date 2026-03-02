// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filterlistimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrefixAlternation(t *testing.T) {
	tests := []struct {
		name   string
		input  []string // pre-sorted
		want   string
		match  []string
		nomatch []string
	}{
		{
			name:    "single word",
			input:   []string{"env"},
			want:    "env",
			match:   []string{"env"},
			nomatch: []string{"en", "envy", ""},
		},
		{
			name:    "no common prefix",
			input:   []string{"env", "host", "pod"},
			want:    "(?:env|host|pod)",
			match:   []string{"env", "host", "pod"},
			nomatch: []string{"en", "ho", "po", "other"},
		},
		{
			name:    "shared first character",
			input:   []string{"env", "error"},
			want:    "e(?:nv|rror)",
			match:   []string{"env", "error"},
			nomatch: []string{"e", "en", "er", "errors"},
		},
		{
			name:    "two groups with shared prefixes",
			input:   []string{"env", "error", "host", "http"},
			want:    "(?:e(?:nv|rror)|h(?:ost|ttp))",
			match:   []string{"env", "error", "host", "http"},
			nomatch: []string{"e", "h", "en", "ho", "other"},
		},
		{
			name:    "word is prefix of another",
			input:   []string{"ab", "abc"},
			want:    "ab(?:c)?",
			match:   []string{"ab", "abc"},
			nomatch: []string{"a", "abcd", ""},
		},
		{
			name:    "shared prefix with optional suffix branch",
			input:   []string{"a", "ab", "ac"},
			want:    "a(?:b|c)?",
			match:   []string{"a", "ab", "ac"},
			nomatch: []string{"", "ad", "abc"},
		},
		{
			name:    "duplicates are deduplicated",
			input:   []string{"env", "env", "host"},
			want:    "(?:env|host)",
			match:   []string{"env", "host"},
			nomatch: []string{"other"},
		},
		{
			name:    "special regex chars are escaped",
			input:   []string{"my.tag", "my.tag2"},
			want:    `my\.tag(?:2)?`,
			match:   []string{"my.tag", "my.tag2"},
			nomatch: []string{"myXtag", "my.tag3"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := prefixAlternation(tc.input)
			assert.Equal(t, tc.want, got)

			// Verify matching behaviour via the full regex.
			re := buildTagRegex(tc.input)
			for _, s := range tc.match {
				assert.True(t, re.MatchString(s), "expected %q to match", s)
			}
			for _, s := range tc.nomatch {
				assert.False(t, re.MatchString(s), "expected %q not to match", s)
			}
		})
	}
}

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

	m1 := matcher.MetricTags["metric1"]
	assert.Equal(t, Exclude, m1.action)
	assert.NotNil(t, m1.tagRegex)
	assert.True(t, m1.tagRegex.MatchString("env"))
	assert.True(t, m1.tagRegex.MatchString("host"))
	assert.False(t, m1.tagRegex.MatchString("pod"))

	m2 := matcher.MetricTags["metric2"]
	assert.Equal(t, Include, m2.action)
	assert.Nil(t, m2.tagRegex)

	m3 := matcher.MetricTags["metric3"]
	assert.Equal(t, Exclude, m3.action)
	assert.NotNil(t, m3.tagRegex)
	assert.True(t, m3.tagRegex.MatchString("pod"))
	assert.False(t, m3.tagRegex.MatchString("env"))
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
