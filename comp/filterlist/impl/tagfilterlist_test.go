// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package filterlistimpl

import (
	"maps"
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

// TestTagMatcherNormalizesMetricNames verifies that metric_tag_filterlist entries
// are keyed on the normalized metric name and looked up by it, so an entry using
// the name as it appears in Datadog strips tags from metrics submitted with the
// raw name the Agent sees.
func TestTagMatcherNormalizesMetricNames(t *testing.T) {
	matcher := newTagMatcher(map[string]MetricTagList{
		// The intake stores this as `my_metric_name`, which is what a user
		// configures, but the Agent sees `my metric-name` on the wire.
		"my_metric_name": {
			Tags:   []string{"env"},
			Action: "exclude",
		},
		// A raw entry is normalized at load time, so it matches too.
		"other metric-name": {
			Tags:   []string{"host"},
			Action: "exclude",
		},
		// Unstorable: no ASCII letter, so it can never match anything.
		"123": {
			Tags:   []string{"pod"},
			Action: "exclude",
		},
	}, logmock.New(t))

	assert.ElementsMatch(t,
		[]string{"my_metric_name", "other_metric_name"},
		slices.Collect(maps.Keys(matcher.MetricTags)),
		"entries should be keyed on the normalized name, dropping unstorable ones")

	// Every name that normalizes to a configured entry strips its tags.
	for _, name := range []string{"my metric-name", "my_metric_name", "my/metric*name"} {
		keepTag, shouldStrip := matcher.ShouldStripTags(name)
		if assert.True(t, shouldStrip, "%q should match its normalized entry", name) {
			assert.False(t, keepTag("env:prod"))
			assert.True(t, keepTag("host:server1"))
		}
	}

	keepTag, shouldStrip := matcher.ShouldStripTags("other metric-name")
	if assert.True(t, shouldStrip, "raw entry should have been normalized at load time") {
		assert.False(t, keepTag("host:server1"))
		assert.True(t, keepTag("env:prod"))
	}

	// Names that normalize to something else are left alone. Periods survive
	// normalization, so this is a different metric entirely.
	_, shouldStrip = matcher.ShouldStripTags("my.metric.name")
	assert.False(t, shouldStrip, "a name that normalizes to a different metric must not match")

	_, shouldStrip = matcher.ShouldStripTags("123")
	assert.False(t, shouldStrip, "unstorable entry must not match")
}

// TestTagMatcherMergesNamesThatNormalizeTogether verifies that entries whose
// names normalize to the same stored name are reconciled the same way duplicate
// entries are, rather than one of them arbitrarily winning.
func TestTagMatcherMergesNamesThatNormalizeTogether(t *testing.T) {
	t.Run("same action merges tags", func(t *testing.T) {
		matcher := newTagMatcher(map[string]MetricTagList{
			"my metric-name": {Tags: []string{"env"}, Action: "exclude"},
			"my_metric_name": {Tags: []string{"host"}, Action: "exclude"},
		}, logmock.New(t))

		require.Len(t, matcher.MetricTags, 1)

		keepTag, shouldStrip := matcher.ShouldStripTags("my metric-name")
		require.True(t, shouldStrip)
		assert.False(t, keepTag("env:prod"))
		assert.False(t, keepTag("host:server1"))
		assert.True(t, keepTag("version:1.0"))
	})

	t.Run("exclude wins over include", func(t *testing.T) {
		for _, metrics := range []map[string]MetricTagList{
			{
				"my metric-name": {Tags: []string{"env"}, Action: "exclude"},
				"my_metric_name": {Tags: []string{"host"}, Action: "include"},
			},
			{
				"my metric-name": {Tags: []string{"host"}, Action: "include"},
				"my_metric_name": {Tags: []string{"env"}, Action: "exclude"},
			},
		} {
			matcher := newTagMatcher(metrics, logmock.New(t))
			require.Len(t, matcher.MetricTags, 1)

			keepTag, shouldStrip := matcher.ShouldStripTags("my_metric_name")
			require.True(t, shouldStrip)
			assert.False(t, keepTag("env:prod"), "the exclude list should be the one in effect")
			assert.True(t, keepTag("host:server1"))
		}
	})
}

// TestTagMatcherLookupDoesNotAllocate pins that normalizing the queried name
// stays allocation free, since the lookup runs for every sample.
func TestTagMatcherLookupDoesNotAllocate(t *testing.T) {
	matcher := newTagMatcher(map[string]MetricTagList{
		"my_metric_name": {Tags: []string{"env"}, Action: "exclude"},
	}, logmock.New(t))

	for _, name := range []string{"my_metric_name", "my metric-name", "unconfigured metric"} {
		t.Run(name, func(t *testing.T) {
			allocs := testing.AllocsPerRun(100, func() {
				matcher.lookup(name)
			})
			assert.Zero(t, allocs)
		})
	}
}

// TestTagMatcherNormalizesTagNames verifies that configured tag names are hashed
// in the same name space as the tags the Agent sees on the wire, so an entry
// written as the tag appears in Datadog (lowercased, `-` and `/` kept) strips the
// raw tag.
func TestTagMatcherNormalizesTagNames(t *testing.T) {
	matcher := newTagMatcher(map[string]MetricTagList{
		"my.metric": {
			// As they appear in Datadog: the Agent sees `My-Tag`,
			// `Kube Namespace` and `Env` on the wire.
			Tags:   []string{"my-tag", "kube_namespace", "env"},
			Action: "exclude",
		},
	}, logmock.New(t))

	keepTag, shouldStrip := matcher.ShouldStripTags("my.metric")
	require.True(t, shouldStrip)

	for _, tag := range []string{
		"My-Tag:value", "my-tag:value", "MY-TAG:value",
		"Kube Namespace:default", "kube_namespace:default", "kube namespace:default",
		"Env:prod", "env:prod",
	} {
		assert.False(t, keepTag(tag), "%q should match its normalized filterlist entry", tag)
	}

	// Tags that normalize to something else are left alone. `-` and `/` survive
	// normalization, so they are not interchangeable with `_`.
	for _, tag := range []string{"my_tag:value", "my.tag:value", "kube-namespace:default", "other:value"} {
		assert.True(t, keepTag(tag), "%q must not match", tag)
	}
}

// TestTagMatcherNormalizesConfiguredTagNames verifies that a raw configured tag
// name is normalized at load time, so it matches too.
func TestTagMatcherNormalizesConfiguredTagNames(t *testing.T) {
	matcher := newTagMatcher(map[string]MetricTagList{
		"my.metric": {
			// `Kube Namespace` normalizes to `kube_namespace`, and `123` is not a
			// storable tag name so it is dropped.
			Tags:   []string{"Kube Namespace", "123"},
			Action: "exclude",
		},
	}, logmock.New(t))

	require.Len(t, matcher.MetricTags["my.metric"].tags, 1, "the unstorable tag name should have been dropped")

	keepTag, shouldStrip := matcher.ShouldStripTags("my.metric")
	require.True(t, shouldStrip)
	assert.False(t, keepTag("kube_namespace:default"))
	assert.False(t, keepTag("Kube Namespace:default"))
	assert.True(t, keepTag("123:whatever"), "a tag the intake drops cannot have been configured")
}

// TestTagMatcherUnstorableTagNameIsNotInTheList verifies that a tag name the intake
// drops is treated as absent from the list, which include and exclude read
// differently, rather than as a blanket keep or strip.
func TestTagMatcherUnstorableTagNameIsNotInTheList(t *testing.T) {
	metrics := map[string]MetricTagList{
		"excluded": {Tags: []string{"env"}, Action: "exclude"},
		"included": {Tags: []string{"env"}, Action: "include"},
	}
	matcher := newTagMatcher(metrics, logmock.New(t))

	keepTag, shouldStrip := matcher.ShouldStripTags("excluded")
	require.True(t, shouldStrip)
	assert.True(t, keepTag("123:x"), "an unconfigurable tag is not excluded")

	keepTag, shouldStrip = matcher.ShouldStripTags("included")
	require.True(t, shouldStrip)
	assert.False(t, keepTag("123:x"), "an unconfigurable tag is not included either")
}

// TestHashTagNameMatchesStringHash pins the assumption hashTagName relies on: the
// byte and string forms of the murmur3 hash agree, so a name hashed after being
// normalized into a buffer matches the same name hashed as a string.
func TestHashTagNameMatchesStringHash(t *testing.T) {
	for _, name := range []string{"env", "my-tag", "kube_namespace", "café"} {
		hashed, ok := hashTagName(name + ":value")
		require.True(t, ok)
		assert.Equal(t, murmur3.StringSum64(name), hashed)
		assert.Equal(t, murmur3.Sum64([]byte(name)), hashed)
	}

	// A name that needs a rewrite hashes to the same value as its normalized form.
	hashed, ok := hashTagName("My Tag:value")
	require.True(t, ok)
	assert.Equal(t, murmur3.StringSum64("my_tag"), hashed)

	_, ok = hashTagName("123:value")
	assert.False(t, ok)
}

// TestHashTagNameTrailingUnderscore pins where the trailing underscore of a tag
// name survives: the intake strips one only from the very end of a tag, so
// `my_tag_:value` is stored with the name `my_tag_`, while the valueless tag
// `my_tag_` is stored as `my_tag`.
func TestHashTagNameTrailingUnderscore(t *testing.T) {
	kept := murmur3.StringSum64("my_tag_")
	stripped := murmur3.StringSum64("my_tag")

	// A value follows the name, so the underscore is part of the name. Both the
	// already normalized name and one that needs a rewrite must agree.
	for _, tag := range []string{"my_tag_:value", "My Tag :value", "my_tag_*:value"} {
		hashed, ok := hashTagName(tag)
		require.True(t, ok, tag)
		assert.Equal(t, kept, hashed, "%q keeps the trailing underscore", tag)
	}

	// The tag carries no value, so the name ends the tag and loses it.
	for _, tag := range []string{"my_tag_", "My Tag ", "my_tag_*"} {
		hashed, ok := hashTagName(tag)
		require.True(t, ok, tag)
		assert.Equal(t, stripped, hashed, "%q loses the trailing underscore", tag)
	}
}

// TestTagMatcherTrailingUnderscore checks the same rule end to end: an entry
// written as the name appears in Datadog strips the tag that carries a value, and
// the valueless form of that tag is a different, unconfigured name.
func TestTagMatcherTrailingUnderscore(t *testing.T) {
	matcher := newTagMatcher(map[string]MetricTagList{
		"my.metric": {Tags: []string{"my_tag_"}, Action: "exclude"},
	}, logmock.New(t))

	keepTag, shouldStrip := matcher.ShouldStripTags("my.metric")
	require.True(t, shouldStrip)

	assert.False(t, keepTag("my_tag_:value"), "the configured name should match")
	assert.False(t, keepTag("My Tag :value"), "the raw form of the configured name should match")
	assert.True(t, keepTag("my_tag:value"), "a different name must not match")
	assert.True(t, keepTag("my_tag_"), "the intake stores this valueless tag as my_tag")

	// Configuring the name of that valueless tag works the other way round.
	matcher = newTagMatcher(map[string]MetricTagList{
		"my.metric": {Tags: []string{"my_tag"}, Action: "exclude"},
	}, logmock.New(t))

	keepTag, shouldStrip = matcher.ShouldStripTags("my.metric")
	require.True(t, shouldStrip)
	assert.False(t, keepTag("my_tag_"), "a valueless tag loses its trailing underscore")
	assert.True(t, keepTag("my_tag_:value"))
}

// TestHashTagNameDoesNotAllocate pins that hashing an observed tag name stays
// allocation free, since it runs for every tag of every sample.
func TestHashTagNameDoesNotAllocate(t *testing.T) {
	for _, tag := range []string{"kube_namespace:default", "Kube Namespace:default", "123:x", "my_tag_", "bare"} {
		t.Run(tag, func(t *testing.T) {
			allocs := testing.AllocsPerRun(100, func() {
				hashTagName(tag)
			})
			assert.Zero(t, allocs)
		})
	}
}

// benchmarkKeepTag measures the per-tag cost of the filter, which runs for every
// tag of every sample whose metric is configured.
func benchmarkKeepTag(b *testing.B, tags []string) {
	matcher := newTagMatcher(map[string]MetricTagList{
		"my.metric": {
			Tags:   []string{"env", "kube_namespace", "my-tag", "host"},
			Action: "exclude",
		},
	}, logmock.New(b))

	keepTag, shouldStrip := matcher.ShouldStripTags("my.metric")
	if !shouldStrip {
		b.Fatal("expected the metric to be configured")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for _, tag := range tags {
			keepTag(tag)
		}
	}
}

func BenchmarkKeepTagNormalized(b *testing.B) {
	benchmarkKeepTag(b, []string{"env:prod", "kube_namespace:default", "service:billing", "version:1.2.3"})
}

func BenchmarkKeepTagNeedsRewrite(b *testing.B) {
	benchmarkKeepTag(b, []string{"Env:prod", "Kube Namespace:default", "Service:billing", "Version:1.2.3"})
}
