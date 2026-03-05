// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	filterlistimpl "github.com/DataDog/datadog-agent/comp/filterlist/impl"
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

func benchmarkTagFilterInEnrich(numTags, numFilterTags int, b *testing.B) {
	conf := enrichConfig{defaultHostname: "hostname"}

	metricTagList := map[string]filterlistimpl.MetricTagList{}
	if numFilterTags > 0 {
		filterTags := make([]string, numFilterTags)
		for i := 0; i < numFilterTags; i++ {
			filterTags[i] = "filter_tag_filter_tag_filter_tag_" + strconv.Itoa(i)
		}
		metricTagList["my.distribution.metric"] = filterlistimpl.MetricTagList{
			Tags:   filterTags,
			Action: "exclude",
		}
	}
	matcher := filterlistimpl.NewTagMatcher(metricTagList)

	// Tags alternate between filter-prefix and non-filter-prefix names
	sampleTags := make([]string, numTags)
	for j := 0; j < numTags; j += 2 {
		sampleTags[j] = "filter_tag_filter_tag_filter_tag_" + strconv.Itoa(j) + ":value"
		sampleTags[j+1] = "foltor_tog_foltor_tog_foltor_tog_" + strconv.Itoa(j) + ":value"
	}

	sample := dogstatsdMetricSample{
		name:       "my.distribution.metric",
		metricType: distributionType,
		tags:       sampleTags,
	}
	out := make([]metrics.MetricSample, 0, 10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out = enrichMetricSample(out[:0], sample, "", 0, "", conf, nil, matcher)
	}
	b.ReportAllocs()
}

// Baseline: no filter rules
func BenchmarkTagFilterInEnrich_NoFilters_10Tags(b *testing.B) {
	benchmarkTagFilterInEnrich(10, 0, b)
}

func BenchmarkTagFilterInEnrich_NoFilters_50Tags(b *testing.B) {
	benchmarkTagFilterInEnrich(50, 0, b)
}

// Small filter list (5 tags to filter)
func BenchmarkTagFilterInEnrich_SmallFilter_10Tags(b *testing.B) {
	benchmarkTagFilterInEnrich(10, 5, b)
}

// Large filter list (50 tags to filter)
func BenchmarkTagFilterInEnrich_LargeFilter_10Tags(b *testing.B) {
	benchmarkTagFilterInEnrich(10, 50, b)
}

// Very large filter list (100 tags to filter)
func BenchmarkTagFilterInEnrich_VeryLargeFilter_10Tags(b *testing.B) {
	benchmarkTagFilterInEnrich(10, 100, b)
}

// Test with more tags in the samples
func BenchmarkTagFilterInEnrich_SmallFilter_50Tags(b *testing.B) {
	benchmarkTagFilterInEnrich(50, 5, b)
}

func BenchmarkTagFilterInEnrich_LargeFilter_50Tags(b *testing.B) {
	benchmarkTagFilterInEnrich(50, 50, b)
}

func BenchmarkTagFilterInEnrich_VeryLargeFilter_50Tags(b *testing.B) {
	benchmarkTagFilterInEnrich(50, 100, b)
}

func BenchmarkMetricsExclusion(b *testing.B) {
	conf := enrichConfig{}

	sample := dogstatsdMetricSample{
		name: "datadog.agent.testing.metric.does_not_match",
	}

	out := make([]metrics.MetricSample, 0, 10)

	b.Run("none", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			enrichMetricSample(out, sample, "", 0, "", conf, nil, nil)
		}
	})

	list := []string{}
	for i := 0; i < 512; i++ {
		list = append(list, fmt.Sprintf("datadog.agent.testing.metric.with_long_name.%d", i))
	}

	for i := 1; i <= 512; i *= 2 {
		matcher := utilstrings.NewMatcher(list[:i], false)
		b.Run(fmt.Sprintf("%d-exact", i),
			func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					enrichMetricSample(out, sample, "", 0, "", conf, &matcher, nil)
				}
			})
	}
}
