// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package strings

import (
	"fmt"
	"math/rand"
	"slices"
	stdstrings "strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewMatcher(t *testing.T) {
	check := func(data []string) []string {
		b := NewMatcher(data, true, nil)
		return b.data
	}

	assert.Equal(t, []string{}, check([]string{}))
	assert.Equal(t, []string{"a"}, check([]string{"a"}))
	assert.Equal(t, []string{"a"}, check([]string{"a", "aa"}))
	assert.Equal(t, []string{"a", "b"}, check([]string{"a", "aa", "b", "bb"}))
	assert.Equal(t, []string{"a", "b"}, check([]string{"a", "b", "bb"}))
}

func TestIsStringMatching(t *testing.T) {
	cases := []struct {
		result      bool
		name        string
		list        []string
		matchPrefix bool
	}{
		{false, "some", []string{}, false},
		{false, "some", []string{}, true},
		{false, "foo", []string{"bar", "baz"}, false},
		{false, "foo", []string{"bar", "baz"}, true},
		{false, "bar", []string{"foo", "baz"}, false},
		{false, "bar", []string{"foo", "baz"}, true},
		{true, "baz", []string{"foo", "baz"}, false},
		{true, "baz", []string{"foo", "baz"}, true},
		{false, "foobar", []string{"foo", "baz"}, false},
		{true, "foobar", []string{"foo", "baz"}, true},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%v-%v-%v", c.name, c.list, c.matchPrefix),
			func(t *testing.T) {
				b := NewMatcher(c.list, c.matchPrefix, nil)
				assert.Equal(t, c.result, b.Test(c.name))
			})
	}
}

func randomString(size uint) string {
	letterBytes := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

	var builder stdstrings.Builder
	for range size {
		builder.WriteByte(letterBytes[rand.Intn(len(letterBytes))])
	}

	return builder.String()
}

func BenchmarkStringsMatcher(b *testing.B) {
	words := []string{
		"foo",
		"longer.name.but.still.small",
		"very.long.string.with.some.good.amount.of.chars.for.a.metric",
		"bar",
	}
	for i := 1000; i <= 10000; i += 1000 {
		b.Run(fmt.Sprintf("strings-matcher-%d", i), func(b *testing.B) {
			var values []string
			for range i {
				values = append(values, randomString(50))
			}
			benchmarkStringsMatcher(b, words, values)
		})
	}
}

func benchmarkStringsMatcher(b *testing.B, words, values []string) {
	b.ReportAllocs()
	b.ResetTimer()

	// first and last will match
	words[0] = values[0]
	words[3] = values[100]

	matcher := NewMatcher(values, false, nil)

	for n := 0; n < b.N; n++ {
		matcher.Test(words[n%len(words)])
	}
}

func TestStripTags(t *testing.T) {
	tests := []struct {
		name         string
		matcherTags  map[string]TagMatcher
		lookupName   string
		inputTags    []string
		expectedTags []string
	}{
		{
			name: "no tags config for name (deny list)",
			matcherTags: map[string]TagMatcher{"metric1": {
				Tags:    []string{"env", "host"},
				Negated: true,
			}},
			lookupName:   "metric2",
			inputTags:    []string{"env:prod", "host:server1", "version:1.0"},
			expectedTags: []string{"env:prod", "host:server1", "version:1.0"},
		},
		{
			name: "strip single tag with value (deny list)",
			matcherTags: map[string]TagMatcher{"metric1": {
				Tags:    []string{"env"},
				Negated: true,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", "host:server1", "version:1.0"},
			expectedTags: []string{"host:server1", "version:1.0"},
		},
		{
			name: "strip multiple tags (deny list)",
			matcherTags: map[string]TagMatcher{"metric1": {
				Tags:    []string{"env", "host"},
				Negated: true,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", "host:server1", "version:1.0", "region:us"},
			expectedTags: []string{"version:1.0", "region:us"},
		},
		{
			name: "strip all tags (deny list)",
			matcherTags: map[string]TagMatcher{"metric1": {
				Tags:    []string{"env", "host", "region", "version"},
				Negated: true,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", "host:server1", "version:1.0", "region:us"},
			expectedTags: []string{},
		},
		{
			name: "no matching tags to strip (deny list)",
			matcherTags: map[string]TagMatcher{"metric1": {
				Tags:    []string{"foo", "bar"},
				Negated: true,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", "host:server1"},
			expectedTags: []string{"env:prod", "host:server1"},
		},
		{
			name: "empty input tags (deny list)",
			matcherTags: map[string]TagMatcher{"metric1": {
				Tags:    []string{"env", "host"},
				Negated: true,
			}},
			lookupName:   "metric1",
			inputTags:    []string{},
			expectedTags: []string{},
		},
		{
			name: "tags without values (deny list)",
			matcherTags: map[string]TagMatcher{"metric1": {
				Tags:    []string{"env", "host"},
				Negated: true,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env", "host:server1", "version"},
			expectedTags: []string{"version"},
		},
		{
			name: "invalid tag starting with colon (deny list)",
			matcherTags: map[string]TagMatcher{"metric1": {
				Tags:    []string{"env", "invalid"},
				Negated: true,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", ":invalid", "host:server1"},
			expectedTags: []string{":invalid", "host:server1"},
		},
		{
			name: "partial tag name match should not strip (deny list)",
			matcherTags: map[string]TagMatcher{"metric1": {
				Tags:    []string{"env"},
				Negated: true,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"environment:prod", "env_var:test", "host:server1"},
			expectedTags: []string{"environment:prod", "env_var:test", "host:server1"},
		},
		{
			name: "tag name with empty value (deny list)",
			matcherTags: map[string]TagMatcher{"metric1": {
				Tags:    []string{"env"},
				Negated: true,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:", "host:server1"},
			expectedTags: []string{"host:server1"},
		},
		{
			name:         "nil matcher tags map",
			matcherTags:  nil,
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", "host:server1"},
			expectedTags: []string{"env:prod", "host:server1"},
		},
		{
			name: "empty tags to strip list (deny list)",
			matcherTags: map[string]TagMatcher{"metric1": {
				Tags:    []string{},
				Negated: true,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", "host:server1"},
			expectedTags: []string{"env:prod", "host:server1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewMatcher([]string{}, false, tt.matcherTags)

			tagMatcher, ok := matcher.ShouldStripTags(tt.lookupName)
			_, tagConfigured := tt.matcherTags[tt.lookupName]
			assert.Equal(t, tagConfigured, ok)

			if ok {
				resultTags, stripped := tagMatcher.StripTags(tt.inputTags)
				assert.Equal(t, tt.expectedTags, resultTags)

				assert.Equal(t, !slices.Equal(resultTags, tt.inputTags), stripped)
			}
		})
	}
}

func TestStripTagsAllowList(t *testing.T) {
	tests := []struct {
		name         string
		matcherTags  map[string]TagMatcher
		lookupName   string
		inputTags    []string
		expectedTags []string
	}{
		{
			name: "keep single tag (allow list)",
			matcherTags: map[string]TagMatcher{"metric1": {
				Tags:    []string{"env"},
				Negated: false,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", "host:server1", "version:1.0"},
			expectedTags: []string{"env:prod"},
		},
		{
			name: "keep multiple tags (allow list)",
			matcherTags: map[string]TagMatcher{"metric1": {
				Tags:    []string{"env", "host"},
				Negated: false,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", "host:server1", "version:1.0", "region:us"},
			expectedTags: []string{"env:prod", "host:server1"},
		},
		{
			name: "keep all tags (allow list)",
			matcherTags: map[string]TagMatcher{"metric1": {
				Tags:    []string{"env", "host", "region", "version"},
				Negated: false,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", "host:server1", "version:1.0", "region:us"},
			expectedTags: []string{"env:prod", "host:server1", "version:1.0", "region:us"},
		},
		{
			name: "no matching tags in allow list",
			matcherTags: map[string]TagMatcher{"metric1": {
				Tags:    []string{"foo", "bar"},
				Negated: false,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", "host:server1"},
			expectedTags: []string{},
		},
		{
			name: "empty input tags (allow list)",
			matcherTags: map[string]TagMatcher{"metric1": {
				Tags:    []string{"env", "host"},
				Negated: false,
			}},
			lookupName:   "metric1",
			inputTags:    []string{},
			expectedTags: []string{},
		},
		{
			name: "tags without values (allow list)",
			matcherTags: map[string]TagMatcher{"metric1": {
				Tags:    []string{"env", "host"},
				Negated: false,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env", "host:server1", "version"},
			expectedTags: []string{"env", "host:server1"},
		},
		{
			name: "invalid tag starting with colon (allow list)",
			matcherTags: map[string]TagMatcher{"metric1": {
				Tags:    []string{"env", "invalid"},
				Negated: false,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", ":invalid", "host:server1"},
			expectedTags: []string{"env:prod"},
		},
		{
			name: "partial tag name match should not keep (allow list)",
			matcherTags: map[string]TagMatcher{"metric1": {
				Tags:    []string{"env"},
				Negated: false,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"environment:prod", "env_var:test", "env:prod", "host:server1"},
			expectedTags: []string{"env:prod"},
		},
		{
			name: "tag name with empty value (allow list)",
			matcherTags: map[string]TagMatcher{"metric1": {
				Tags:    []string{"env"},
				Negated: false,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:", "host:server1"},
			expectedTags: []string{"env:"},
		},
		{
			name: "empty allow list strips all tags",
			matcherTags: map[string]TagMatcher{"metric1": {
				Tags:    []string{},
				Negated: false,
			}},
			lookupName:   "metric1",
			inputTags:    []string{"env:prod", "host:server1"},
			expectedTags: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewMatcher([]string{}, false, tt.matcherTags)

			tagMatcher, ok := matcher.ShouldStripTags(tt.lookupName)
			_, tagConfigured := tt.matcherTags[tt.lookupName]
			assert.Equal(t, tagConfigured, ok)

			if ok {
				resultTags, stripped := tagMatcher.StripTags(tt.inputTags)
				assert.Equal(t, tt.expectedTags, resultTags)

				assert.Equal(t, !slices.Equal(resultTags, tt.inputTags), stripped)
			}
		})
	}
}

func TestStripTagsDoesNotMutate(t *testing.T) {
	// Test that StripTags does not mutate the original slice
	matcherTags := map[string]TagMatcher{"metric1": {
		Tags:    []string{"env"},
		Negated: true,
	}}
	matcher := NewMatcher([]string{}, false, matcherTags)

	inputTags := []string{"env:prod", "host:server1", "version:1.0"}

	tagMatcher, ok := matcher.ShouldStripTags("metric1")

	assert.True(t, ok, "metric should need tag stripping")

	resultTags, stripped := tagMatcher.StripTags(inputTags)

	assert.True(t, stripped, "tags should be stripped")
	assert.Equal(t, []string{"host:server1", "version:1.0"}, resultTags)

	// The original tags should be unchanged
	assert.Equal(t, []string{"env:prod", "host:server1", "version:1.0"}, inputTags)
}

func TestStripTagsMutModifiesInPlace(t *testing.T) {
	// Test that StripTagsMut modifies the slice in place and returns a re-sliced version
	matcherTags := map[string]TagMatcher{"metric1": {
		Tags:    []string{"env"},
		Negated: true,
	}}
	matcher := NewMatcher([]string{}, false, matcherTags)

	inputTags := []string{"env:prod", "host:server1", "version:1.0"}
	originalCap := cap(inputTags)

	tagMatcher, ok := matcher.ShouldStripTags("metric1")

	assert.True(t, ok, "metric should need tag stripping")

	resultTags, stripped := tagMatcher.StripTagsMut(inputTags)

	assert.True(t, stripped, "tags should be stripped")
	assert.Equal(t, []string{"host:server1", "version:1.0"}, resultTags)

	assert.Equal(t, originalCap, cap(resultTags), "capacity should be preserved")
}
