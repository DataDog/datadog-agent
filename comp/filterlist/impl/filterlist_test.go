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

	filtered := filterList.createHistogramsFilterList(bl)
	require.ElementsMatch(filtered, []string{"foo.avg", "foo.max", "baz.73percentile", "bar.22percentile"})
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

// TestNormalizeMetricNamesDropsUnstorable verifies the helper drops names the
// intake would reject outright rather than keeping them as dead entries.
func TestNormalizeMetricNamesDropsUnstorable(t *testing.T) {
	require := require.New(t)

	in := []string{"valid.metric", "", "123", "...", "another.valid"}
	out := normalizeMetricNames(in, logmock.New(t))

	require.Equal([]string{"valid.metric", "another.valid"}, out)
}
