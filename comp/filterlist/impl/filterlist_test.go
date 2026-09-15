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

func TestHistogramMetricNamesFilter(t *testing.T) {
	cfg := make(map[string]interface{})
	require := require.New(t)

	cfg["histogram_aggregates"] = []string{"avg", "max", "median"}
	cfg["histogram_percentiles"] = []string{"0.73", "0.22"}
	bl := []string{
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
	}
	cfg["metric_filterlist"] = bl

	logComponent := logmock.New(t)
	configComponent := config.NewMockWithOverrides(t, cfg)
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	filterList := NewFilterList(logComponent, configComponent, telemetryComponent)

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

func TestHistogramMetricNamesFilterWithPrefixes(t *testing.T) {
	require := require.New(t)

	bl := []string{
		"foo",         // exact, no aggregate suffix
		"foo.avg",     // exact, aggregate suffix
		"bar.*",       // prefix: can match bar.histo.avg
		"baz.9*",      // prefix: can match baz.95percentile
		"qux.avg.*",   // prefix
		"count.other", // exact, no aggregate suffix
	}

	newFilterList := func(matchPrefix bool) *FilterList {
		cfg := map[string]interface{}{
			"histogram_aggregates":           []string{"avg", "max", "median"},
			"histogram_percentiles":          []string{"0.73"},
			"metric_filterlist":              bl,
			"metric_filterlist_match_prefix": matchPrefix,
		}
		logComponent := logmock.New(t)
		configComponent := config.NewMockWithOverrides(t, cfg)
		telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
		return NewFilterList(logComponent, configComponent, telemetryComponent)
	}

	// Every prefix entry has to be kept: it can match an aggregate-suffixed name
	// that the exact suffix check cannot recognise.
	histo := newFilterList(false).GetHistoFilterList()
	require.True(histo.Test("foo.avg"), "exact entry with an aggregate suffix should be kept")
	require.True(histo.Test("bar.anything"), "bar.* prefix should be kept unconditionally")
	require.True(histo.Test("baz.95percentile"), "baz.9* prefix should be kept unconditionally")
	require.True(histo.Test("qux.avg.x"), "qux.avg.* prefix should be kept unconditionally")
	require.False(histo.Test("foo"), "exact entry without an aggregate suffix should not be kept")
	require.False(histo.Test("count.other"), "exact entry without an aggregate suffix should not be kept")

	// With the legacy global prefix mode, every entry becomes a prefix, so the
	// histogram filter list is compiled identically to the main one.
	filterListPrefix := newFilterList(true)
	main := filterListPrefix.GetMetricFilterList()
	histoPrefix := filterListPrefix.GetHistoFilterList()
	for _, name := range []string{"foo", "fooX", "foo.avg", "bar.anything", "baz.95percentile", "qux.avg.x", "count.other", "count.otherX"} {
		require.Equal(main.Test(name), histoPrefix.Test(name), "%s should match the histogram list iff it matches the main list", name)
	}
}

func TestMetricFilterListPrefixEntries(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["metric_filterlist"] = []string{"exact.metric", "prefixed.metric.*"}

	logComponent := logmock.New(t)
	configComponent := config.NewMockWithOverrides(t, cfg)
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	filterList := NewFilterList(logComponent, configComponent, telemetryComponent)

	matcher := filterList.GetMetricFilterList()

	require.True(t, matcher.Test("exact.metric"))
	require.False(t, matcher.Test("exact.metric.suffix"))
	require.True(t, matcher.Test("prefixed.metric."))
	require.True(t, matcher.Test("prefixed.metric.anything"))
	require.False(t, matcher.Test("prefixed.metric"))
	require.False(t, matcher.Test("other.metric"))

	// A prefix entry is kept in the histogram subset, so it also applies to the
	// aggregates derived at flush time.
	histo := filterList.GetHistoFilterList()
	require.True(t, histo.Test("prefixed.metric.histo.avg"))
	require.False(t, histo.Test("exact.metric"))
}

func TestMetricFilterListLegacyBlocklistPrefixEntries(t *testing.T) {
	cfg := make(map[string]interface{})
	// The deprecated setting is used when metric_filterlist is empty, and
	// supports the same per-entry prefixes.
	cfg["statsd_metric_blocklist"] = []string{"legacy.*"}

	logComponent := logmock.New(t)
	configComponent := config.NewMockWithOverrides(t, cfg)
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	filterList := NewFilterList(logComponent, configComponent, telemetryComponent)

	matcher := filterList.GetMetricFilterList()
	require.True(t, matcher.Test("legacy.metric"))
	require.False(t, matcher.Test("other.metric"))
}

// newFilterListWithMetricPrefixList builds a filterlist over the raw
// `metric_filterlist_prefix` value a config file would decode to.
func newFilterListWithMetricPrefixList(t *testing.T, list interface{}) *FilterList {
	cfg := map[string]interface{}{"metric_filterlist_prefix": list}

	logComponent := logmock.New(t)
	configComponent := config.NewMockWithOverrides(t, cfg)
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	return NewFilterList(logComponent, configComponent, telemetryComponent)
}

// TestMetricFilterListPrefixKeyIsAlwaysAPrefix pins that every
// metric_filterlist_prefix entry is a prefix, whether or not it ends with
// `*` -- unlike metric_filterlist, which needs metric_filterlist_match_prefix
// or a trailing `*` for that.
func TestMetricFilterListPrefixKeyIsAlwaysAPrefix(t *testing.T) {
	filterList := newFilterListWithMetricPrefixList(t, []interface{}{"plain", "withstar.*"})

	matcher := filterList.GetMetricFilterList()
	require.True(t, matcher.Test("plain"))
	require.True(t, matcher.Test("plainX"), "no trailing `*` needed: metric_filterlist_prefix entries are always prefixes")
	require.True(t, matcher.Test("withstar."))
	require.True(t, matcher.Test("withstar.anything"))
}

func TestMetricFilterListPrefixKeyExceptions(t *testing.T) {
	filterList := newFilterListWithMetricPrefixList(t, []interface{}{
		"plain.*",
		map[string]interface{}{
			"metric_name": "redis.*",
			"except": []interface{}{
				"redis.net.commands",
				"redis.keys.*",
			},
		},
	})

	matcher := filterList.GetMetricFilterList()

	// Entries without exceptions are unaffected.
	require.True(t, matcher.Test("plain.anything"))

	// The prefix is dropped...
	require.True(t, matcher.Test("redis."))
	require.True(t, matcher.Test("redis.mem.used"))
	require.True(t, matcher.Test("redis.net.commands.rate"))
	// ...except for the metrics it excepts, exact or prefix.
	require.False(t, matcher.Test("redis.net.commands"))
	require.False(t, matcher.Test("redis.keys."))
	require.False(t, matcher.Test("redis.keys.count"))
	// An exception does not widen the entry.
	require.False(t, matcher.Test("redis"))
	require.False(t, matcher.Test("other.metric"))

	// Exceptions also survive into the histogram subset applied at flush time.
	histo := filterList.GetHistoFilterList()
	require.True(t, histo.Test("redis.latency.avg"))
	require.False(t, histo.Test("redis.keys.count.avg"))
}

// TestMetricFilterListPrefixKeyExceptionsAreScopedToTheirEntry pins that an
// exception narrows only the entry declaring it: a name another entry
// matches is still dropped.
func TestMetricFilterListPrefixKeyExceptionsAreScopedToTheirEntry(t *testing.T) {
	filterList := newFilterListWithMetricPrefixList(t, []interface{}{
		map[string]interface{}{
			"metric_name": "foo.*",
			"except":      []interface{}{"foo.keep", "foo.bar.keep"},
		},
		"foo.bar.*",
	})

	matcher := filterList.GetMetricFilterList()
	require.False(t, matcher.Test("foo.keep"))
	// Excepted by `foo.*`, but `foo.bar.*` matches it unconditionally.
	require.True(t, matcher.Test("foo.bar.keep"))
}

// TestMetricFilterListPrefixKeyExceptionsFromYAML exercises the real YAML
// decoding path, which is the only way the object form of an entry reaches
// the Agent.
func TestMetricFilterListPrefixKeyExceptionsFromYAML(t *testing.T) {
	configComponent := config.NewMockFromYAML(t, `
metric_filterlist_prefix:
  - plain.metric
  - plain.prefix.*
  - metric_name: redis.*
    except:
      - redis.net.commands
      - redis.keys.*
`)

	logComponent := logmock.New(t)
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	filterList := NewFilterList(logComponent, configComponent, telemetryComponent)

	matcher := filterList.GetMetricFilterList()
	require.Equal(t, 3, matcher.Len())
	require.True(t, matcher.Test("plain.metric"))
	// `plain.metric` has no trailing `*` in the config, but this key treats
	// every entry as a prefix regardless.
	require.True(t, matcher.Test("plain.metric.suffix"))
	require.True(t, matcher.Test("plain.prefix.anything"))
	require.True(t, matcher.Test("redis.mem.used"))
	require.False(t, matcher.Test("redis.net.commands"))
	require.False(t, matcher.Test("redis.keys.count"))
}

func TestMetricFilterListPrefixKeyMalformedEntries(t *testing.T) {
	cases := []struct {
		name  string
		entry interface{}
	}{
		{"unknown field", map[string]interface{}{"metric_name": "foo.*", "excepts": []interface{}{"foo.keep"}}},
		{"missing metric name", map[string]interface{}{"except": []interface{}{"foo.keep"}}},
		{"empty metric name", map[string]interface{}{"metric_name": ""}},
		{"not a name nor an object", []interface{}{"foo.*"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// A malformed entry is skipped, and does not take the rest of the
			// list -- or the Agent -- down with it.
			filterList := newFilterListWithMetricPrefixList(t, []interface{}{c.entry, "valid.metric"})

			matcher := filterList.GetMetricFilterList()
			require.Equal(t, 1, matcher.Len())
			require.True(t, matcher.Test("valid.metric"))
		})
	}
}

// TestMetricFilterListCombinesBothKeys pins that metric_filterlist and
// metric_filterlist_prefix compile into a single matcher, each keeping its
// own semantics: metric_filterlist stays exact unless matchPrefix or a
// trailing `*` says otherwise, metric_filterlist_prefix is always a prefix.
func TestMetricFilterListCombinesBothKeys(t *testing.T) {
	cfg := map[string]interface{}{
		"metric_filterlist": []string{"exact.only"},
		"metric_filterlist_prefix": []interface{}{
			map[string]interface{}{
				"metric_name": "prefix.only",
				"except":      []interface{}{"prefix.only.keep"},
			},
		},
	}

	logComponent := logmock.New(t)
	configComponent := config.NewMockWithOverrides(t, cfg)
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	filterList := NewFilterList(logComponent, configComponent, telemetryComponent)

	matcher := filterList.GetMetricFilterList()

	// metric_filterlist's entry is exact: no trailing `*`, no match_prefix.
	require.True(t, matcher.Test("exact.only"))
	require.False(t, matcher.Test("exact.only.suffix"))

	// metric_filterlist_prefix's entry is a prefix regardless, and keeps its
	// exception.
	require.True(t, matcher.Test("prefix.only.anything"))
	require.False(t, matcher.Test("prefix.only.keep"))

	require.False(t, matcher.Test("unrelated"))
}

func TestMetricFilterListGlobalMatchPrefixStripsStar(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["metric_filterlist"] = []string{"foo.*", "bar"}
	cfg["metric_filterlist_match_prefix"] = true

	logComponent := logmock.New(t)
	configComponent := config.NewMockWithOverrides(t, cfg)
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	filterList := NewFilterList(logComponent, configComponent, telemetryComponent)

	matcher := filterList.GetMetricFilterList()
	// `foo.*` is a prefix on `foo.`, not on the literal `foo.*`.
	require.True(t, matcher.Test("foo.metric"))
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

	logComponent := logmock.New(t)
	configComponent := config.NewMockWithOverrides(t, cfg)
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	filterList := NewFilterList(logComponent, configComponent, telemetryComponent)

	matcher := filterList.GetMetricFilterList()

	// The raw submitted name normalizes to the stored entry, so it is filtered.
	require.True(matcher.Test("my metric-name"), "raw name should match its normalized filterlist entry")
	require.True(matcher.Test("my_metric_name"), "normalized name should match")
	require.True(matcher.Test("already_normalized.metric"), "already-normalized entry should match")

	// An unstorable entry cannot match anything.
	require.False(matcher.Test("123"), "unstorable entry must not match")
	require.False(matcher.Test("unrelated.metric"), "unrelated metric must not match")
}

// TestMetricFilterListNormalizesPrefixEntries verifies that normalization and
// per-entry prefixes compose: the prefix of a raw entry is normalized, and the
// entry keeps matching by prefix.
func TestMetricFilterListNormalizesPrefixEntries(t *testing.T) {
	require := require.New(t)

	cfg := map[string]interface{}{
		// Normalizes to the prefix entry `my_metric.*`.
		"metric_filterlist": []string{"my metric.*"},
	}

	logComponent := logmock.New(t)
	configComponent := config.NewMockWithOverrides(t, cfg)
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	filterList := NewFilterList(logComponent, configComponent, telemetryComponent)

	matcher := filterList.GetMetricFilterList()
	require.True(matcher.Test("my metric.count"), "raw name should match the normalized prefix")
	require.True(matcher.Test("my_metric.count"), "normalized name should match the prefix")
	require.True(matcher.Test("my_metric."), "the prefix itself should match")
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
	// `123.*` can never match a stored name and is dropped; `service_` only
	// keeps its boundary when the whole list is prefixes.
	in := []string{"my metric-name.*", "123.*", "service_", "exact"}

	require.Equal(
		[]string{"my_metric_name.*", "service", "exact"},
		normalizeMetricNames("metric_filterlist", in, false, logComponent),
	)
	require.Equal(
		[]string{"my_metric_name.*", "service_", "exact"},
		normalizeMetricNames("metric_filterlist", in, true, logComponent),
	)
}

// TestMetricFilterListPrefixBoundaryIsNotWidened is the end-to-end form of
// TestNormalizeMetricNamesKeepsPrefixBoundary: `service_*` must drop the
// `service_` family only, and leave `service.requests` alone.
func TestMetricFilterListPrefixBoundaryIsNotWidened(t *testing.T) {
	for name, cfg := range map[string]map[string]interface{}{
		"per-entry prefix": {
			"metric_filterlist": []string{"service_*"},
		},
		"global match prefix": {
			"metric_filterlist":              []string{"service_"},
			"metric_filterlist_match_prefix": true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			require := require.New(t)

			logComponent := logmock.New(t)
			configComponent := config.NewMockWithOverrides(t, cfg)
			telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
			filterList := NewFilterList(logComponent, configComponent, telemetryComponent)

			matcher := filterList.GetMetricFilterList()

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
