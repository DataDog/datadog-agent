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

// TestKeepTagNormalizesKeys checks that tag keys are compared in the form the
// intake stores them in.
func TestKeepTagNormalizesKeys(t *testing.T) {
	exclude := NewHashedMetricTagList(true, []string{"env", "pod_name", "foo_", ""})
	for _, tag := range []string{"env:prod", "Env:Prod", "ENV", "env_", "env:", "Pod Name:x", "pod!name:x", "foo_:bar", ":foo", "123:foo"} {
		assert.False(t, exclude.KeepTag(tag), "tag %q should be stripped", tag)
	}
	// Keys that only look similar, or that the intake stores differently.
	for _, tag := range []string{"envx:prod", "en:v", "foo_", "foo:bar", "pod-name:x", "version:1.0"} {
		assert.True(t, exclude.KeepTag(tag), "tag %q should be kept", tag)
	}
}

// TestShouldStripTagsNormalizesNames checks that metric names are compared in
// the form the intake stores them in, keeping their case.
func TestShouldStripTagsNormalizesNames(t *testing.T) {
	m := NewTagMatcher(map[string]HashedMetricTagList{
		"my_metric.count": NewHashedMetricTagList(true, []string{"env"}),
	})
	for _, name := range []string{"my_metric.count", "my-metric.count", "my metric.count", "1my_metric.count"} {
		_, ok := m.ShouldStripTags(name)
		assert.True(t, ok, "metric name %q", name)
	}
	for _, name := range []string{"my_metric.counts", "My_metric.count", "other", "123"} {
		_, ok := m.ShouldStripTags(name)
		assert.False(t, ok, "metric name %q", name)
	}
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

	// Matching does not allocate, even when normalizing a tag key.
	for _, tag := range []string{"env:prod", "ENV:prod"} {
		allocs := testing.AllocsPerRun(100, func() {
			tl, ok := m.ShouldStripTags("metric")
			if !ok || tl.KeepTag(tag) {
				t.Fatal("unexpected match result")
			}
		})
		assert.Zero(t, allocs, "tag %q", tag)
	}

	// Normalizing a name never allocates.
	for _, name := range []string{"Metric", "metric-x", "123"} {
		assert.Zero(t, testing.AllocsPerRun(100, func() { _, _ = m.ShouldStripTags(name) }), "name %q", name)
	}
}
