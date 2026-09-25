// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filterlistimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetrynoop "github.com/DataDog/datadog-agent/comp/core/telemetry/fx-noop"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/require"
)

// newTestFilterList builds a FilterList from a plain config override map,
// exactly as agent configuration/RC would produce it: a
// metric_filterlist_prefix entry is a `[]map[string]interface{}`, not a Go
// struct, since that is the shape a real (YAML- or JSON-sourced) config value
// takes.
func newTestFilterList(t *testing.T, cfg map[string]interface{}) *FilterList {
	logComponent := logmock.New(t)
	configComponent := config.NewMockWithOverrides(t, cfg)
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	return NewFilterList(logComponent, configComponent, telemetryComponent)
}

// prefixListConfig turns a list of MetricPrefixListEntry into the
// map-of-interfaces shape the configuration tree actually stores, for use as
// a `metric_filterlist_prefix` override in tests.
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

// TestHistogramMetricNamesFilterWithPrefixes checks that both prefix
// mechanisms -- metric_filterlist_prefix entries and the legacy
// metric_filterlist_match_prefix whole-list mode -- are kept in the
// histogram-specific filter list, since a prefix can always match an
// aggregate-suffixed name the exact-suffix check cannot recognise on its own.
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

	// With the legacy global prefix mode, every metric_filterlist entry
	// becomes a prefix too, so the histogram filter list is compiled
	// identically to the main one.
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

// TestMetricFilterListPrefixListEntries is the end-to-end test of
// metric_filterlist_prefix: an entry is always a prefix, `*` or not, and can
// declare exceptions.
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

	// A prefix rule (with its exceptions) is kept in the histogram subset, so
	// it also applies to the aggregates derived at flush time.
	histo := filterList.GetHistoFilterList()
	require.True(histo.Test("prefixed.metric.histo.avg"))
	require.False(histo.Test("prefixed.metric.keep"))
	require.False(histo.Test("exact.metric"))
}

// TestMetricFilterListStarIsNoLongerAPrefixMarker asserts the reverted
// behavior: unlike before, a metric_filterlist (or the deprecated
// statsd_metric_blocklist) entry ending with `*` is not a prefix pattern
// anymore. `*` is just a literal, non-matchable character there; a per-entry
// prefix belongs in metric_filterlist_prefix instead.
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

			// The trailing `*` normalizes away like any other trailing
			// non-alphanumeric, non-period byte would, leaving a plain exact
			// entry -- it is no longer a prefix marker.
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

	// `foo*` still has no special-cased `*`: matchPrefix normalizes it like
	// any other prefix ending in a non-alphanumeric, non-period byte, which
	// keeps the boundary as an underscore rather than widening to `foo`.
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

// TestMetricFilterListPrefixNormalizesEntries verifies that normalization and
// metric_filterlist_prefix compose: the prefix of a raw entry is normalized,
// and the entry keeps matching by prefix -- including its exceptions.
func TestMetricFilterListPrefixNormalizesEntries(t *testing.T) {
	require := require.New(t)

	cfg := map[string]interface{}{
		// Normalizes to the prefix `my_metric.`.
		"metric_filterlist_prefix": prefixListConfig(MetricPrefixListEntry{
			Prefix: "my metric.",
			// Normalizes to `my_metric.count`.
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
	// `123` can never match a stored name and is dropped; `service_` only
	// keeps its boundary when the whole list is prefixes.
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

// TestMetricFilterListPrefixBoundaryIsNotWidened is the end-to-end form of
// TestNormalizeMetricNames's boundary-preserving behavior, checked through
// both ways of getting a prefix: metric_filterlist_prefix, and the legacy
// metric_filterlist_match_prefix whole-list mode. `service_` must drop the
// `service_` family only, and leave `service.requests` alone.
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

// TestMetricFilterListPrefixNoSegmentBoundaryRequired asserts that the Agent
// does not require a metric_filterlist_prefix entry to end on a segment
// boundary: `sys` legitimately matches both `system.cpu` and `sys.cpu`.
func TestMetricFilterListPrefixNoSegmentBoundaryRequired(t *testing.T) {
	cfg := map[string]interface{}{
		"metric_filterlist_prefix": prefixListConfig(MetricPrefixListEntry{Prefix: "sys"}),
	}
	matcher := newTestFilterList(t, cfg).GetMetricFilterList()

	require.True(t, matcher.Test("system.cpu"))
	require.True(t, matcher.Test("sys.cpu"))
	require.False(t, matcher.Test("other.cpu"))
}

// TestMetricFilterListPrefixEmptyPrefixMatchesAll asserts that an empty
// metric_filterlist_prefix prefix is valid configuration, not something
// dropped: it matches every metric name.
func TestMetricFilterListPrefixEmptyPrefixMatchesAll(t *testing.T) {
	cfg := map[string]interface{}{
		"metric_filterlist_prefix": prefixListConfig(MetricPrefixListEntry{Prefix: ""}),
	}
	matcher := newTestFilterList(t, cfg).GetMetricFilterList()

	require.True(t, matcher.Test("anything"))
	require.True(t, matcher.Test("other.metric"))
}

// TestMetricFilterListPrefixDeadRuleIsDropped asserts that a
// metric_filterlist_prefix entry whose exceptions can never take effect,
// because a broader prefix already matches everything it could ever match,
// is dropped -- but the broader prefix still blocks the metric.
func TestMetricFilterListPrefixDeadRuleIsDropped(t *testing.T) {
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

	require.True(t, matcher.Test("postgresql.locks.waiting"), "the dead rule's exception has no effect")
}
