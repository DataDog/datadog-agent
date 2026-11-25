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

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics/metricname"
)

func TestNewTagMatcher(t *testing.T) {
	matcher := NewTagMatcher(map[string]MetricTagList{
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

	list, ok := matcher.ShouldStripTags("metric1")
	assert.True(t, ok)
	assert.Equal(t, metricname.NewHashedMetricTagList(true, []string{"env", "host"}), list)

	list, ok = matcher.ShouldStripTags("metric2")
	assert.True(t, ok)
	assert.Equal(t, metricname.NewHashedMetricTagList(false, []string{}), list)

	list, ok = matcher.ShouldStripTags("metric3")
	assert.True(t, ok)
	assert.Equal(t, metricname.NewHashedMetricTagList(true, []string{"pod"}), list)
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

	matcher := NewTagMatcher(metrics, logmock.New(t))

	// Test metric1 tags are excluded
	tagList, shouldStrip := matcher.ShouldStripTags("metric1")
	assert.True(t, shouldStrip)

	assert.False(t, tagList.KeepTag("env:prod"))
	assert.False(t, tagList.KeepTag("host:server1"))
	assert.True(t, tagList.KeepTag("version:1.0"))

	// Test metric2 tags are included
	tagList, shouldStrip = matcher.ShouldStripTags("metric2")
	assert.True(t, shouldStrip)

	assert.True(t, tagList.KeepTag("env:prod"))
	assert.True(t, tagList.KeepTag("host:server1"))
	assert.False(t, tagList.KeepTag("version:1.0"))

	// Test metric3 tags are all excluded
	tagList, shouldStrip = matcher.ShouldStripTags("metric3")
	assert.True(t, shouldStrip)

	assert.False(t, tagList.KeepTag("env:prod"))
	assert.False(t, tagList.KeepTag("host:server1"))
	assert.False(t, tagList.KeepTag("version:1.0"))

	// Test metric4 tags are all included
	tagList, shouldStrip = matcher.ShouldStripTags("metric4")
	if shouldStrip { // 2 behaviors are acceptable: return true with a function that keeps all tags, or return false
		assert.True(t, tagList.KeepTag("env:prod"))
		assert.True(t, tagList.KeepTag("host:server1"))
		assert.True(t, tagList.KeepTag("version:1.0"))
	}

	// metric5 is not configured
	_, shouldStrip = matcher.ShouldStripTags("metric5")
	assert.False(t, shouldStrip)
}

// TestCompileTagFilterListNormalizesRules checks that rules are compiled in the
// form the intake stores names and tag keys in.
func TestCompileTagFilterListNormalizesRules(t *testing.T) {
	matcher := compileTagFilterList([]MetricTagListEntry{
		{MetricName: "My-Metric", Action: "exclude", Tags: []string{"Env", "pod name", "123", "a:b", "foo_"}},
		{MetricName: "123", Action: "exclude", Tags: []string{"env"}},
		{MetricName: "", Action: "exclude", Tags: []string{"env"}},
	}, logmock.New(t))

	assert.Equal(t, 1, matcher.Len())
	list, ok := matcher.ShouldStripTags("My_Metric")
	assert.True(t, ok)
	// "123" is the empty stored key, as in `123:foo` (stored as `:foo`); "a:b" cannot be a key.
	assert.Equal(t, metricname.NewHashedMetricTagList(true, []string{"env", "pod_name", "", "foo_"}), list)
}

// TestCompileTagFilterListMergesNormalizedNames checks that rules naming the same
// metric once normalized are merged, regardless of their order.
func TestCompileTagFilterListMergesNormalizedNames(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []MetricTagListEntry
		want    metricname.HashedMetricTagList
	}{
		{
			name: "same action merges tags",
			entries: []MetricTagListEntry{
				{MetricName: "a.b-c", Action: "include", Tags: []string{"env"}},
				{MetricName: "a.b_c", Action: "include", Tags: []string{"Host", "env"}},
			},
			want: metricname.NewHashedMetricTagList(false, []string{"env", "host"}),
		},
		{
			name: "exclude wins",
			entries: []MetricTagListEntry{
				{MetricName: "a.b-c", Action: "include", Tags: []string{"env"}},
				{MetricName: "a.b c", Action: "exclude", Tags: []string{"pod"}},
				{MetricName: "a.b_c", Action: "", Tags: []string{"host"}},
				{MetricName: "a.b__c", Action: "include", Tags: []string{"version"}},
			},
			want: metricname.NewHashedMetricTagList(true, []string{"pod", "host"}),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, entries := range [][]MetricTagListEntry{tc.entries, reversed(tc.entries)} {
				matcher := compileTagFilterList(entries, logmock.New(t))
				assert.Equal(t, 1, matcher.Len())
				list, ok := matcher.ShouldStripTags("a.b_c")
				assert.True(t, ok)
				assert.Equal(t, tc.want, list)
			}
		})
	}
}

// TestTagMatcherMatchesNormalizedSamples checks the compiled matcher end to end:
// rules and samples in different raw forms of the same stored name and key.
func TestTagMatcherMatchesNormalizedSamples(t *testing.T) {
	matcher := NewTagMatcher(map[string]MetricTagList{
		"my_metric.count": {Tags: []string{"Pod Name"}, Action: "exclude"},
	}, logmock.New(t))

	list, ok := matcher.ShouldStripTags("my-metric.count")
	assert.True(t, ok)
	assert.False(t, list.KeepTag("pod_name:web"))
	// `-` is kept in tag keys, so this is a different key.
	assert.True(t, list.KeepTag("pod-name:web"))
	assert.True(t, list.KeepTag("env:prod"))
}

func reversed[T any](s []T) []T {
	r := slices.Clone(s)
	slices.Reverse(r)
	return r
}
