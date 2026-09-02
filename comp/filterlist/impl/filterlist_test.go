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
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
	"github.com/stretchr/testify/require"
)

// metricRules turns plain metric names into the rules the filterlist works
// with, none of them carrying exceptions.
func metricRules(names ...string) []utilstrings.Rule {
	rules := make([]utilstrings.Rule, 0, len(names))
	for _, name := range names {
		rules = append(rules, utilstrings.Rule{Pattern: name})
	}
	return rules
}

func TestHistogramMetricNamesFilter(t *testing.T) {
	cfg := make(map[string]interface{})
	require := require.New(t)

	cfg["histogram_aggregates"] = []string{"avg", "max", "median"}
	cfg["histogram_percentiles"] = []string{"0.73", "0.22"}

	logComponent := logmock.New(t)
	configComponent := config.NewMockWithOverrides(t, cfg)
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	filterList := NewFilterList(logComponent, configComponent, telemetryComponent)

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

	filtered := filterList.createHistogramsFilterList(metricRules(bl...), false)
	require.ElementsMatch(filtered, metricRules("foo.avg", "foo.max", "baz.73percentile", "bar.22percentile"))
}

func TestHistogramMetricNamesFilterWithPrefixes(t *testing.T) {
	cfg := make(map[string]interface{})
	require := require.New(t)

	cfg["histogram_aggregates"] = []string{"avg", "max", "median"}
	cfg["histogram_percentiles"] = []string{"0.73"}

	logComponent := logmock.New(t)
	configComponent := config.NewMockWithOverrides(t, cfg)
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	filterList := NewFilterList(logComponent, configComponent, telemetryComponent)

	bl := []string{
		"foo",         // exact, no aggregate suffix
		"foo.avg",     // exact, aggregate suffix
		"bar.*",       // prefix: can match bar.histo.avg
		"baz.9*",      // prefix: can match baz.95percentile
		"qux.avg.*",   // prefix
		"count.other", // exact, no aggregate suffix
	}

	// Every prefix entry has to be kept: it can match an aggregate-suffixed name
	// that the exact suffix check cannot recognise.
	filtered := filterList.createHistogramsFilterList(metricRules(bl...), false)
	require.ElementsMatch(filtered, metricRules("foo.avg", "bar.*", "baz.9*", "qux.avg.*"))

	// With the legacy global prefix mode, every entry is a prefix.
	filtered = filterList.createHistogramsFilterList(metricRules(bl...), true)
	require.ElementsMatch(filtered, metricRules(bl...))
}

// TestHistogramMetricNamesFilterKeepsExceptions checks that a kept prefix entry
// keeps its exceptions: a name excepted before aggregation has to stay excepted
// once the aggregate suffix is appended.
func TestHistogramMetricNamesFilterKeepsExceptions(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["histogram_aggregates"] = []string{"avg", "max"}

	logComponent := logmock.New(t)
	configComponent := config.NewMockWithOverrides(t, cfg)
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	filterList := NewFilterList(logComponent, configComponent, telemetryComponent)

	rules := []utilstrings.Rule{
		{Pattern: "histo.*", Except: []string{"histo.keep.avg"}},
		{Pattern: "exact.avg", Except: []string{"never.matches"}},
		{Pattern: "exact.no.aggregate"},
	}

	filtered := filterList.createHistogramsFilterList(rules, false)
	require.ElementsMatch(t, rules[:2], filtered)
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

// newFilterListWithMetricList builds a filterlist over the raw
// `metric_filterlist` value a config file would decode to.
func newFilterListWithMetricList(t *testing.T, list interface{}) *FilterList {
	cfg := map[string]interface{}{"metric_filterlist": list}

	logComponent := logmock.New(t)
	configComponent := config.NewMockWithOverrides(t, cfg)
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	return NewFilterList(logComponent, configComponent, telemetryComponent)
}

func TestMetricFilterListExceptions(t *testing.T) {
	// The shape a config file decodes to: plain names and objects mixed in the
	// same list.
	filterList := newFilterListWithMetricList(t, []interface{}{
		"plain.metric",
		"plain.prefix.*",
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
	require.True(t, matcher.Test("plain.metric"))
	require.True(t, matcher.Test("plain.prefix.anything"))

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

// TestMetricFilterListExceptionsAreScopedToTheirEntry pins that an exception
// narrows only the entry declaring it: a name another entry matches is still
// dropped.
func TestMetricFilterListExceptionsAreScopedToTheirEntry(t *testing.T) {
	filterList := newFilterListWithMetricList(t, []interface{}{
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

// TestMetricFilterListExceptionsFromYAML exercises the real YAML decoding path,
// which is the only way the object form of an entry reaches the Agent.
func TestMetricFilterListExceptionsFromYAML(t *testing.T) {
	configComponent := config.NewMockFromYAML(t, `
metric_filterlist:
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
	require.True(t, matcher.Test("plain.prefix.anything"))
	require.True(t, matcher.Test("redis.mem.used"))
	require.False(t, matcher.Test("redis.net.commands"))
	require.False(t, matcher.Test("redis.keys.count"))
}

func TestMetricFilterListMalformedEntries(t *testing.T) {
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
			filterList := newFilterListWithMetricList(t, []interface{}{c.entry, "valid.metric"})

			matcher := filterList.GetMetricFilterList()
			require.Equal(t, 1, matcher.Len())
			require.True(t, matcher.Test("valid.metric"))
		})
	}
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
