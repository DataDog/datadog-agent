// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package filterlistimpl

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/murmur3"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
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
	}, logmock.New(t))

	metric1Tags := []uint64{murmur3.StringSum64("env"), murmur3.StringSum64("host")}
	slices.Sort(metric1Tags)

	assert.NotNil(t, matcher)
	assert.Equal(t, matcher.MetricTags["metric1"], hashedMetricTagList{
		tags:   metric1Tags,
		action: exclude,
	})

	assert.Equal(t, matcher.MetricTags["metric2"], hashedMetricTagList{
		tags:   []uint64{},
		action: include,
	})

	assert.Equal(t, matcher.MetricTags["metric3"], hashedMetricTagList{
		tags:   []uint64{murmur3.StringSum64("pod")},
		action: exclude,
	})
}

// TestTagMatcherNormalizesSamples checks that samples are matched in the form
// the intake stores them in, both for the metric name and for tag keys.
func TestTagMatcherNormalizesSamples(t *testing.T) {
	matcher := newTagMatcher(map[string]MetricTagList{
		"my_metric.count": {Tags: []string{"env", "pod_name", "foo_"}, Action: "exclude"},
	}, logmock.New(t))

	for _, name := range []string{"my_metric.count", "my-metric.count", "my metric.count", "1my_metric.count"} {
		keep, ok := matcher.ShouldStripTags(name)
		require.True(t, ok, "metric name %q", name)

		for _, tag := range []string{"env:prod", "Env:Prod", "ENV", "env_", "env:", "pod_name:x", "Pod_Name:x", "pod name:x", "pod!name:x", "foo_:bar"} {
			assert.False(t, keep(tag), "metric %q, tag %q should be stripped", name, tag)
		}
		// Keys that only look similar, or that the intake stores differently.
		for _, tag := range []string{"envx:prod", "en:v", "foo_", "foo:bar", "pod:x", "pod-name:x", ":env", "123:env", "version:1.0"} {
			assert.True(t, keep(tag), "metric %q, tag %q should be kept", name, tag)
		}
	}

	for _, name := range []string{"my_metric.counts", "My_metric.count", "other", "my_metric", "123"} {
		_, ok := matcher.ShouldStripTags(name)
		assert.False(t, ok, "metric name %q", name)
	}
}

// TestCompileTagFilterListNormalizesRules checks that rules are compiled in the
// form the intake stores names and tag keys in.
func TestCompileTagFilterListNormalizesRules(t *testing.T) {
	matcher := compileTagFilterList([]MetricTagListEntry{
		{MetricName: "My-Metric", Action: "exclude", Tags: []string{"Env", "pod name", "123", "a:b", "foo_"}},
		{MetricName: "123", Action: "exclude", Tags: []string{"env"}},
		{MetricName: "", Action: "exclude", Tags: []string{"env"}},
	}, logmock.New(t))

	require.Len(t, matcher.MetricTags, 1)
	// "123" is the empty stored key, as in `123:foo` (stored as `:foo`); "a:b" cannot be a key.
	assert.Equal(t, newHashedMetricTagList(exclude, hashTags([]string{"env", "pod_name", "", "foo_"})), matcher.MetricTags["My_Metric"])
}

// TestCompileTagFilterListMergesNormalizedNames checks that rules naming the same
// metric once normalized are merged, regardless of their order.
func TestCompileTagFilterListMergesNormalizedNames(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []MetricTagListEntry
		want    hashedMetricTagList
	}{
		{
			name: "same action merges tags",
			entries: []MetricTagListEntry{
				{MetricName: "a.b-c", Action: "include", Tags: []string{"env"}},
				{MetricName: "a.b_c", Action: "include", Tags: []string{"Host", "env"}},
			},
			want: newHashedMetricTagList(include, hashTags([]string{"env", "host"})),
		},
		{
			name: "exclude wins",
			entries: []MetricTagListEntry{
				{MetricName: "a.b-c", Action: "include", Tags: []string{"env"}},
				{MetricName: "a.b c", Action: "exclude", Tags: []string{"pod"}},
				{MetricName: "a.b_c", Action: "", Tags: []string{"host"}},
				{MetricName: "a.b__c", Action: "include", Tags: []string{"version"}},
			},
			want: newHashedMetricTagList(exclude, hashTags([]string{"pod", "host"})),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, entries := range [][]MetricTagListEntry{tc.entries, reversed(tc.entries)} {
				matcher := compileTagFilterList(entries, logmock.New(t))
				require.Len(t, matcher.MetricTags, 1)
				assert.Equal(t, tc.want, matcher.MetricTags["a.b_c"])
			}
		})
	}
}

func reversed[T any](s []T) []T {
	r := slices.Clone(s)
	slices.Reverse(r)
	return r
}

func TestShouldStripTagsDoesNotAllocate(t *testing.T) {
	matcher := newTagMatcher(map[string]MetricTagList{
		"my_metric": {Tags: []string{"env"}, Action: "exclude"},
	}, logmock.New(t))
	empty := newTagMatcher(nil, logmock.New(t))

	assert.Zero(t, testing.AllocsPerRun(100, func() { _, _ = empty.ShouldStripTags("My-Metric") }))

	keep, _ := matcher.ShouldStripTags("my_metric")
	for _, tag := range []string{"env:prod", "kube_namespace:default", "host"} {
		assert.Zero(t, testing.AllocsPerRun(100, func() { _ = keep(tag) }), "tag %q", tag)
	}
	// Tags the intake rewrites cost one allocation for their normalized key.
	for _, tag := range []string{"Env:Prod", "\u00c9quipe:x"} {
		assert.LessOrEqual(t, testing.AllocsPerRun(100, func() { _ = keep(tag) }), 1.0, "tag %q", tag)
	}
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

	matcher := newTagMatcher(metrics, logmock.New(t))

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

// TestTagMatcherEmptyKey checks that a rule for the empty key matches the tags
// the intake stores with an empty key, such as `:foo`.
func TestTagMatcherEmptyKey(t *testing.T) {
	matcher := newTagMatcher(map[string]MetricTagList{
		"metric": {Tags: []string{""}, Action: "exclude"},
	}, logmock.New(t))

	keep, ok := matcher.ShouldStripTags("metric")
	require.True(t, ok)
	for _, tag := range []string{":foo", "123:foo", "_:foo", ":"} {
		assert.False(t, keep(tag), "tag %q should be stripped", tag)
	}
	for _, tag := range []string{"foo", "foo:bar", "a:"} {
		assert.True(t, keep(tag), "tag %q should be kept", tag)
	}
}
