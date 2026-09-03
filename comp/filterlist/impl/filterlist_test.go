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

// TestTagFilterListNormalizesEntries verifies that metric_tag_filterlist entries
// loaded from the config are keyed on the normalized metric name, so an entry
// written as the metric appears in Datadog strips tags from metrics submitted
// with the raw name.
func TestTagFilterListNormalizesEntries(t *testing.T) {
	require := require.New(t)

	cfg := map[string]interface{}{
		"metric_tag_filterlist": []map[string]interface{}{
			{
				// `my metric-name` normalizes to this, the name a user sees.
				"metric_name": "my_metric_name",
				"action":      "exclude",
				"tags":        []string{"env"},
			},
			{
				// A raw entry is normalized at load time too.
				"metric_name": "other metric-name",
				"action":      "exclude",
				"tags":        []string{"host"},
			},
			{
				// Unstorable: no ASCII letter, so it can never match.
				"metric_name": "123",
				"action":      "exclude",
				"tags":        []string{"pod"},
			},
		},
	}

	logComponent := logmock.New(t)
	configComponent := config.NewMockWithOverrides(t, cfg)
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	filterList := NewFilterList(logComponent, configComponent, telemetryComponent)

	matcher := filterList.GetTagFilterList()

	// The raw submitted name normalizes to the configured entry, so its tags
	// are stripped.
	keepTag, shouldStrip := matcher.ShouldStripTags("my metric-name")
	require.True(shouldStrip, "raw name should match its normalized filterlist entry")
	require.False(keepTag("env:prod"))
	require.True(keepTag("host:server1"))

	keepTag, shouldStrip = matcher.ShouldStripTags("other_metric_name")
	require.True(shouldStrip, "raw entry should have been normalized at load time")
	require.False(keepTag("host:server1"))

	_, shouldStrip = matcher.ShouldStripTags("123")
	require.False(shouldStrip, "unstorable entry must not match")

	_, shouldStrip = matcher.ShouldStripTags("unrelated.metric")
	require.False(shouldStrip, "unrelated metric must not match")
}

// TestTagFilterListNormalizesTagNames verifies that the tag names of
// metric_tag_filterlist entries loaded from the config are normalized, so an entry
// written as the tag appears in Datadog strips the raw tag the Agent sees.
func TestTagFilterListNormalizesTagNames(t *testing.T) {
	require := require.New(t)

	cfg := map[string]interface{}{
		"metric_tag_filterlist": []map[string]interface{}{
			{
				"metric_name": "my.metric",
				"action":      "exclude",
				// As they appear in Datadog, plus one raw entry and one the
				// intake would drop.
				"tags": []string{"kube_namespace", "my-tag", "Raw Tag", "123"},
			},
		},
	}

	logComponent := logmock.New(t)
	configComponent := config.NewMockWithOverrides(t, cfg)
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	filterList := NewFilterList(logComponent, configComponent, telemetryComponent)

	keepTag, shouldStrip := filterList.GetTagFilterList().ShouldStripTags("my.metric")
	require.True(shouldStrip)

	require.False(keepTag("Kube Namespace:default"), "raw tag should match its normalized entry")
	require.False(keepTag("kube_namespace:default"))
	require.False(keepTag("My-Tag:value"))
	require.False(keepTag("raw_tag:value"), "raw entry should have been normalized at load time")
	require.True(keepTag("unrelated:value"))
	require.True(keepTag("123:value"), "unstorable entry must not match")
}
