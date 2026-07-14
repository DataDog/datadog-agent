// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scraper

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetricFilterExactStringInclude(t *testing.T) {
	f, err := NewMetricFilter(
		[]interface{}{"go_goroutines"},
		nil,
		nil,
		nil,
		false,
	)
	assert.NoError(t, err)

	match, ok := f.MatchMetric("go_goroutines")
	assert.True(t, ok)
	assert.Equal(t, "go_goroutines", match.Name)
	assert.Equal(t, "native", match.Type)

	_, ok = f.MatchMetric("http_requests")
	assert.False(t, ok)
}

func TestMetricFilterRegexInclude(t *testing.T) {
	f, err := NewMetricFilter(
		[]interface{}{"go_.*"},
		nil,
		nil,
		nil,
		false,
	)
	assert.NoError(t, err)

	match, ok := f.MatchMetric("go_goroutines")
	assert.True(t, ok)
	assert.Equal(t, "go_goroutines", match.Name)

	match, ok = f.MatchMetric("go_threads")
	assert.True(t, ok)
	assert.Equal(t, "go_threads", match.Name)

	_, ok = f.MatchMetric("http_requests")
	assert.False(t, ok)
}

func TestMetricFilterMapIncludeWithRename(t *testing.T) {
	f, err := NewMetricFilter(
		[]interface{}{
			map[string]interface{}{"go_goroutines": "runtime.goroutines"},
		},
		nil,
		nil,
		nil,
		false,
	)
	assert.NoError(t, err)

	match, ok := f.MatchMetric("go_goroutines")
	assert.True(t, ok)
	assert.Equal(t, "runtime.goroutines", match.Name)
	assert.Equal(t, "native", match.Type)
}

func TestMetricFilterMapIncludeWithTypeOverride(t *testing.T) {
	f, err := NewMetricFilter(
		[]interface{}{
			map[string]interface{}{
				"my_counter": map[string]interface{}{
					"name": "app.counter",
					"type": "counter",
				},
			},
		},
		nil,
		nil,
		nil,
		false,
	)
	assert.NoError(t, err)

	match, ok := f.MatchMetric("my_counter")
	assert.True(t, ok)
	assert.Equal(t, "app.counter", match.Name)
	assert.Equal(t, "counter", match.Type)
}

func TestMetricFilterMatchAllWildcard(t *testing.T) {
	f, err := NewMetricFilter(
		[]interface{}{".*"},
		nil,
		nil,
		nil,
		false,
	)
	assert.NoError(t, err)

	match, ok := f.MatchMetric("anything_at_all")
	assert.True(t, ok)
	assert.Equal(t, "anything_at_all", match.Name)
	assert.Equal(t, "native", match.Type)

	match, ok = f.MatchMetric("some_other_metric")
	assert.True(t, ok)
	assert.Equal(t, "some_other_metric", match.Name)
}

func TestMetricFilterExcludeExact(t *testing.T) {
	f, err := NewMetricFilter(
		[]interface{}{".*"},
		nil,
		[]string{"unwanted_metric"},
		nil,
		false,
	)
	assert.NoError(t, err)

	_, ok := f.MatchMetric("unwanted_metric")
	assert.False(t, ok)

	match, ok := f.MatchMetric("wanted_metric")
	assert.True(t, ok)
	assert.Equal(t, "wanted_metric", match.Name)
}

func TestMetricFilterExcludeRegex(t *testing.T) {
	f, err := NewMetricFilter(
		[]interface{}{".*"},
		nil,
		[]string{"internal_.*"},
		nil,
		false,
	)
	assert.NoError(t, err)

	_, ok := f.MatchMetric("internal_debug")
	assert.False(t, ok)

	_, ok = f.MatchMetric("internal_stats")
	assert.False(t, ok)

	match, ok := f.MatchMetric("external_metric")
	assert.True(t, ok)
	assert.Equal(t, "external_metric", match.Name)
}

func TestMetricFilterExcludeTakesPrecedenceOverInclude(t *testing.T) {
	f, err := NewMetricFilter(
		[]interface{}{"go_goroutines"},
		nil,
		[]string{"go_goroutines"},
		nil,
		false,
	)
	assert.NoError(t, err)

	_, ok := f.MatchMetric("go_goroutines")
	assert.False(t, ok, "exclude should take precedence over include")
}

func TestShouldExcludeSampleMatchingLabels(t *testing.T) {
	f, err := NewMetricFilter(
		[]interface{}{".*"},
		nil,
		nil,
		map[string][]string{
			"env": {"staging"},
		},
		false,
	)
	assert.NoError(t, err)

	excluded := f.ShouldExcludeSample(map[string]string{
		"env": "staging",
	})
	assert.True(t, excluded)
}

func TestShouldExcludeSampleWildcardValue(t *testing.T) {
	f, err := NewMetricFilter(
		[]interface{}{".*"},
		nil,
		nil,
		map[string][]string{
			"env": {"*"},
		},
		false,
	)
	assert.NoError(t, err)

	excluded := f.ShouldExcludeSample(map[string]string{
		"env": "production",
	})
	assert.True(t, excluded, "wildcard '*' should match any label value")

	excluded = f.ShouldExcludeSample(map[string]string{
		"env": "staging",
	})
	assert.True(t, excluded, "wildcard '*' should match any label value")
}

func TestShouldExcludeSampleNonMatchingLabels(t *testing.T) {
	f, err := NewMetricFilter(
		[]interface{}{".*"},
		nil,
		nil,
		map[string][]string{
			"env": {"staging"},
		},
		false,
	)
	assert.NoError(t, err)

	excluded := f.ShouldExcludeSample(map[string]string{
		"env": "production",
	})
	assert.False(t, excluded)

	excluded = f.ShouldExcludeSample(map[string]string{
		"region": "us-east-1",
	})
	assert.False(t, excluded, "label not in exclude_by_labels should not be excluded")
}

func TestMetricFilterCachedRegexResultsAreConsistent(t *testing.T) {
	f, err := NewMetricFilter(
		[]interface{}{"go_.*"},
		nil,
		nil,
		nil,
		true, // enable caching
	)
	assert.NoError(t, err)

	// First call populates the cache.
	match1, ok1 := f.MatchMetric("go_goroutines")
	assert.True(t, ok1)

	// Second call reads from cache.
	match2, ok2 := f.MatchMetric("go_goroutines")
	assert.True(t, ok2)

	assert.Equal(t, match1, match2, "cached result should be identical to original")

	// Verify a non-matching metric is also consistently cached.
	_, ok3 := f.MatchMetric("http_requests")
	assert.False(t, ok3)

	_, ok4 := f.MatchMetric("http_requests")
	assert.False(t, ok4, "cached non-match should remain false")
}
