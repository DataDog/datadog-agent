// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filterlistimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetrynoop "github.com/DataDog/datadog-agent/comp/core/telemetry/fx-noop"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/require"
)

// Config overrides mimic decoded YAML/RC, not Go structs.
func newTestFilterList(t *testing.T, cfg map[string]interface{}) *FilterList {
	logComponent := logmock.New(t)
	configComponent := config.NewMockWithOverrides(t, cfg)
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	return NewFilterList(logComponent, configComponent, telemetryComponent)
}

func prefixListConfig(entries ...MetricPrefixListEntry) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(entries))
	for _, e := range entries {
		out = append(out, map[string]interface{}{
			"prefix":        e.Prefix,
			"except_prefix": e.ExceptPrefix,
			"except_exact":  e.ExceptExact,
		})
	}
	return out
}

func TestHistogramMetricNamesFilter(t *testing.T) {
	require := require.New(t)

	cfg := map[string]interface{}{
		"histogram_aggregates":  []string{"avg", "max", "median"},
		"histogram_percentiles": []string{"0.73", "0.22"},
		"metric_filterlist": []string{
			"foo",
			"bar",
			"baz",
			"foomax",
			"foo.avg",
			"foo.max",
			"foo.count",
			"baz.73percentile",
			"bar.50percentile",
			"bar.22percentile",
			"count",
		},
	}
	filterList := newTestFilterList(t, cfg)

	histo := filterList.GetHistoFilterList()

	// Only names ending in a configured histogram aggregate or percentile
	// suffix belong in the histogram-specific filter list.
	for _, kept := range []string{"foo.avg", "foo.max", "baz.73percentile", "bar.22percentile"} {
		require.True(histo.Test(kept), "%s should be in the histogram filter list", kept)
	}
	for _, dropped := range []string{"foo", "bar", "baz", "foomax", "foo.count", "bar.50percentile", "count"} {
		require.False(histo.Test(dropped), "%s should not be in the histogram filter list", dropped)
	}
}

// Prefixes can match histogram aggregate names synthesized at flush time.
func TestHistogramMetricNamesFilterWithPrefixes(t *testing.T) {
	require := require.New(t)

	exact := []string{
		"foo",         // exact, no aggregate suffix
		"foo.avg",     // exact, aggregate suffix
		"count.other", // exact, no aggregate suffix
	}
	prefixes := []MetricPrefixListEntry{
		{Prefix: "bar."},     // can match bar.histo.avg
		{Prefix: "baz.9"},    // can match baz.95percentile
		{Prefix: "qux.avg."}, // can match qux.avg.x
	}

	histo := newTestFilterList(t, map[string]interface{}{
		"histogram_aggregates":     []string{"avg", "max", "median"},
		"histogram_percentiles":    []string{"0.73"},
		"metric_filterlist":        exact,
		"metric_filterlist_prefix": prefixListConfig(prefixes...),
	}).GetHistoFilterList()

	require.True(histo.Test("foo.avg"), "exact entry with an aggregate suffix should be kept")
	require.True(histo.Test("bar.anything"), "metric_filterlist_prefix entry should be kept unconditionally")
	require.True(histo.Test("baz.95percentile"), "metric_filterlist_prefix entry should be kept unconditionally")
	require.True(histo.Test("qux.avg.x"), "metric_filterlist_prefix entry should be kept unconditionally")
	require.False(histo.Test("foo"), "exact entry without an aggregate suffix should not be kept")
	require.False(histo.Test("count.other"), "exact entry without an aggregate suffix should not be kept")

	// Global prefix mode makes main and histogram matchers share prefix behavior.
	filterListPrefix := newTestFilterList(t, map[string]interface{}{
		"histogram_aggregates":           []string{"avg", "max", "median"},
		"histogram_percentiles":          []string{"0.73"},
		"metric_filterlist":              exact,
		"metric_filterlist_match_prefix": true,
	})
	main := filterListPrefix.GetMetricFilterList()
	histoPrefix := filterListPrefix.GetHistoFilterList()
	for _, name := range []string{"foo", "fooX", "foo.avg", "count.other", "count.otherX"} {
		require.Equal(main.Test(name), histoPrefix.Test(name), "%s should match the histogram list iff it matches the main list", name)
	}
}

func TestMetricFilterListPrefixListEntries(t *testing.T) {
	require := require.New(t)

	cfg := map[string]interface{}{
		"metric_filterlist": []string{"exact.metric"},
		"metric_filterlist_prefix": prefixListConfig(
			MetricPrefixListEntry{
				Prefix:       "prefixed.metric.",
				ExceptExact:  []string{"prefixed.metric.keep"},
				ExceptPrefix: []string{"prefixed.metric.locks."},
			},
		),
	}
	filterList := newTestFilterList(t, cfg)

	matcher := filterList.GetMetricFilterList()

	require.True(matcher.Test("exact.metric"))
	require.False(matcher.Test("exact.metric.suffix"))
	require.True(matcher.Test("prefixed.metric."))
	require.True(matcher.Test("prefixed.metric.anything"))
	require.False(matcher.Test("prefixed.metric.keep"), "except_exact entry should be kept")
	require.False(matcher.Test("prefixed.metric.locks.waiting"), "except_prefix entry should be kept")
	require.False(matcher.Test("prefixed.metric"))
	require.False(matcher.Test("other.metric"))

	// Prefix rules also apply to histogram aggregates.
	histo := filterList.GetHistoFilterList()
	require.True(histo.Test("prefixed.metric.histo.avg"))
	require.False(histo.Test("prefixed.metric.keep"))
	require.False(histo.Test("exact.metric"))
}

// `*` in metric_filterlist is not a prefix marker.
func TestMetricFilterListStarIsNoLongerAPrefixMarker(t *testing.T) {
	for name, cfg := range map[string]map[string]interface{}{
		"metric_filterlist": {
			"metric_filterlist": []string{"exactfoo*"},
		},
		"statsd_metric_blocklist": {
			"statsd_metric_blocklist": []string{"exactfoo*"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			matcher := newTestFilterList(t, cfg).GetMetricFilterList()

			// `*` normalizes away instead of widening the entry.
			require.True(t, matcher.Test("exactfoo"))
			require.False(t, matcher.Test("exactfooX"), "no longer widened into a prefix on exactfoo")
			require.False(t, matcher.Test("exactfoo."), "no longer widened into a prefix on exactfoo")
		})
	}
}

func TestMetricFilterListGlobalMatchPrefixOnStarEntry(t *testing.T) {
	cfg := map[string]interface{}{
		"metric_filterlist":              []string{"foo*", "bar"},
		"metric_filterlist_match_prefix": true,
	}
	matcher := newTestFilterList(t, cfg).GetMetricFilterList()

	// matchPrefix still preserves the normalized `foo_` boundary.
	require.True(t, matcher.Test("foo_metric"))
	require.False(t, matcher.Test("foo.metric"), "a different family than foo_...")
	require.False(t, matcher.Test("foo"))
	// The global flag still turns a plain entry into a prefix.
	require.True(t, matcher.Test("bar.metric"))
}

// TestMetricFilterListNormalizesEntries verifies that metric_filterlist entries
// are normalized at load time, so a raw entry such as `my metric-name` (which
// the intake stores as `my_metric_name`) matches metrics submitted with that
// raw name. Without normalization the verbatim entry would never match, since
// Matcher.Test normalizes the query name but not the list.
func TestMetricFilterListNormalizesEntries(t *testing.T) {
	require := require.New(t)

	cfg := map[string]interface{}{
		// `my metric-name` normalizes to `my_metric_name`; `123` is unstorable
		// (no ASCII letter) and must be dropped.
		"metric_filterlist": []string{"my metric-name", "123", "already_normalized.metric"},
	}

	matcher := newTestFilterList(t, cfg).GetMetricFilterList()

	// The raw submitted name normalizes to the stored entry, so it is filtered.
	require.True(matcher.Test("my metric-name"), "raw name should match its normalized filterlist entry")
	require.True(matcher.Test("my_metric_name"), "normalized name should match")
	require.True(matcher.Test("already_normalized.metric"), "already-normalized entry should match")

	// An unstorable entry cannot match anything.
	require.False(matcher.Test("123"), "unstorable entry must not match")
	require.False(matcher.Test("unrelated.metric"), "unrelated metric must not match")
}

func TestMetricFilterListPrefixNormalizesEntries(t *testing.T) {
	require := require.New(t)

	cfg := map[string]interface{}{
		"metric_filterlist_prefix": prefixListConfig(MetricPrefixListEntry{
			Prefix: "my metric.",
			// Exact exceptions use full-name normalization.
			ExceptExact: []string{"my metric.count"},
		}),
	}

	matcher := newTestFilterList(t, cfg).GetMetricFilterList()
	require.True(matcher.Test("my metric.other"), "raw name should match the normalized prefix")
	require.True(matcher.Test("my_metric.other"), "normalized name should match the prefix")
	require.True(matcher.Test("my_metric."), "the prefix itself should match")
	require.False(matcher.Test("my_metric.count"), "except_exact entry should be kept")
	require.False(matcher.Test("my_metric"), "shorter than the prefix, should not match")
	require.False(matcher.Test("other.metric"))
}

// TestNormalizeMetricNames verifies the wiring around
// metricname.NormalizeEntries, which owns the entry format and is tested there:
// the prefix mode is passed through, and unusable entries are dropped from the
// list the component keeps rather than reported some other way.
func TestNormalizeMetricNames(t *testing.T) {
	require := require.New(t)

	logComponent := logmock.New(t)
	// `service_` keeps its boundary only in prefix mode.
	in := []string{"my metric-name", "123", "service_", "exact"}

	require.Equal(
		[]string{"my_metric_name", "service", "exact"},
		normalizeMetricNames(in, false, logComponent),
	)
	require.Equal(
		[]string{"my_metric_name", "service_", "exact"},
		normalizeMetricNames(in, true, logComponent),
	)
}

// `service_` must not widen to `service.` through either prefix path.
func TestMetricFilterListPrefixBoundaryIsNotWidened(t *testing.T) {
	for name, cfg := range map[string]map[string]interface{}{
		"metric_filterlist_prefix": {
			"metric_filterlist_prefix": prefixListConfig(MetricPrefixListEntry{Prefix: "service_"}),
		},
		"global match prefix": {
			"metric_filterlist":              []string{"service_"},
			"metric_filterlist_match_prefix": true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			require := require.New(t)

			matcher := newTestFilterList(t, cfg).GetMetricFilterList()

			// The family the entry names, submitted raw or normalized.
			require.True(matcher.Test("service_requests"))
			require.True(matcher.Test("service requests"))
			require.True(matcher.Test("service-requests"))

			// Anything past that boundary is a different metric.
			require.False(matcher.Test("service.requests"), "the period boundary is a different family")
			require.False(matcher.Test("services.requests"))
			require.False(matcher.Test("serviceother"))
			require.False(matcher.Test("service"))
		})
	}
}

func TestMetricFilterListPrefixNoSegmentBoundaryRequired(t *testing.T) {
	cfg := map[string]interface{}{
		"metric_filterlist_prefix": prefixListConfig(MetricPrefixListEntry{Prefix: "sys"}),
	}
	matcher := newTestFilterList(t, cfg).GetMetricFilterList()

	require.True(t, matcher.Test("system.cpu"))
	require.True(t, matcher.Test("sys.cpu"))
	require.False(t, matcher.Test("other.cpu"))
}

func TestMetricFilterListPrefixEmptyPrefixMatchesAll(t *testing.T) {
	cfg := map[string]interface{}{
		"metric_filterlist_prefix": prefixListConfig(MetricPrefixListEntry{Prefix: ""}),
	}
	matcher := newTestFilterList(t, cfg).GetMetricFilterList()

	require.True(t, matcher.Test("anything"))
	require.True(t, matcher.Test("other.metric"))
}

func TestMetricFilterListPrefixExceptionsApplyAcrossRules(t *testing.T) {
	cfg := map[string]interface{}{
		"metric_filterlist_prefix": prefixListConfig(
			MetricPrefixListEntry{Prefix: "postgresql."},
			MetricPrefixListEntry{
				Prefix:      "postgresql.locks.",
				ExceptExact: []string{"postgresql.locks.waiting"},
			},
		),
	}
	matcher := newTestFilterList(t, cfg).GetMetricFilterList()

	require.False(t, matcher.Test("postgresql.locks.waiting"))
	require.True(t, matcher.Test("postgresql.locks.blocked"))
}
