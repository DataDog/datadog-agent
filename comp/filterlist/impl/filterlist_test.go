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

	filtered := filterList.createHistogramsFilterList(bl, false)
	require.ElementsMatch(filtered, []string{"foo.avg", "foo.max", "baz.73percentile", "bar.22percentile"})
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
	filtered := filterList.createHistogramsFilterList(bl, false)
	require.ElementsMatch(filtered, []string{"foo.avg", "bar.*", "baz.9*", "qux.avg.*"})

	// With the legacy global prefix mode, every entry is a prefix.
	filtered = filterList.createHistogramsFilterList(bl, true)
	require.ElementsMatch(filtered, bl)
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
