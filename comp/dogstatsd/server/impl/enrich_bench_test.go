// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverimpl

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/pkg/metricpipelines/names"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

func buildTags(tagCount int) []string {
	tags := make([]string, 0, tagCount)
	for i := 0; i < tagCount; i++ {
		tags = append(tags, fmt.Sprintf("tag%d:val%d", i, i))
	}

	return tags
}

// used to store the result and avoid optimizations
var tags []string

func BenchmarkExtractTagsMetadata(b *testing.B) {
	conf := enrichConfig{
		defaultHostname: "hostname",
	}
	for i := 20; i <= 200; i += 20 {
		b.Run(fmt.Sprintf("%d-tags", i), func(sb *testing.B) {
			baseTags := append([]string{hostTagPrefix + "foo", entityIDTagPrefix + "bar"}, buildTags(i/10)...)
			sb.ResetTimer()

			for n := 0; n < sb.N; n++ {
				tags, _, _, _ = extractTagsMetadata(baseTags, "", 0, origindetection.LocalData{}, origindetection.ExternalData{}, "", conf)
			}
		})
	}
}

func BenchmarkMetricsExclusion(b *testing.B) {
	conf := enrichConfig{}

	sample := dogstatsdMetricSample{
		name: "datadog.agent.testing.metric.does_not_match",
	}

	out := make([]metrics.MetricSample, 0, 10)

	b.Run("none", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			enrichMetricSample(out, sample, "", 0, "", conf, nil)
		}
	})

	list := []string{}
	for i := 0; i < 512; i++ {
		list = append(list, fmt.Sprintf("datadog.agent.testing.metric.with_long_name.%d", i))
	}

	for i := 1; i <= 512; i *= 2 {
		matcher := utilstrings.NewBlocklistMatcher(list[:i], false)
		filters := names.NewTestFilters(names.CriterionMetricFilterList, matcher, utilstrings.Matcher{})
		b.Run(fmt.Sprintf("%d-exact", i),
			func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					enrichMetricSample(out, sample, "", 0, "", conf, &filters)
				}
			})
	}
}

func BenchmarkMetricsInclusion(b *testing.B) {
	conf := enrichConfig{}

	nonMatchingSample := dogstatsdMetricSample{
		name: "datadog.agent.testing.metric.does_not_match",
	}
	matchingSample := dogstatsdMetricSample{
		name: "datadog.agent.testing.metric.with_long_name.0",
	}

	out := make([]metrics.MetricSample, 0, 10)

	b.Run("none", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			enrichMetricSample(out, nonMatchingSample, "", 0, "", conf, nil)
		}
	})

	list := make([]string, 0, 512)
	for i := 0; i < 512; i++ {
		list = append(list, fmt.Sprintf("datadog.agent.testing.metric.with_long_name.%d", i))
	}

	for i := 1; i <= 512; i *= 2 {
		matcher := utilstrings.NewAllowlistMatcher(list[:i], false)
		filters := names.NewTestFilters(names.CriterionCloudCostMetrics, utilstrings.Matcher{}, matcher)
		b.Run(fmt.Sprintf("%d-exact-not-in-list", i),
			func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					enrichMetricSample(out, nonMatchingSample, "", 0, "", conf, &filters)
				}
			})
	}

	for i := 1; i <= 512; i *= 2 {
		matcher := utilstrings.NewAllowlistMatcher(list[:i], true)
		filters := names.NewTestFilters(names.CriterionCloudCostMetrics, utilstrings.Matcher{}, matcher)
		b.Run(fmt.Sprintf("%d-prefix-not-in-list", i),
			func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					enrichMetricSample(out, nonMatchingSample, "", 0, "", conf, &filters)
				}
			})
	}

	matcher := utilstrings.NewAllowlistMatcher(list, false)
	filters := names.NewTestFilters(names.CriterionCloudCostMetrics, utilstrings.Matcher{}, matcher)
	b.Run("512-exact-in-list", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			enrichMetricSample(out, matchingSample, "", 0, "", conf, &filters)
		}
	})
}
