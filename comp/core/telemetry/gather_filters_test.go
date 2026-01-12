// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build !serverless

package telemetry

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoFilter(t *testing.T) {
	tests := []struct {
		name       string
		metricName string
		expected   bool
	}{
		{
			name:       "accepts any metric",
			metricName: "any_metric_name",
			expected:   true,
		},
		{
			name:       "accepts empty metric name",
			metricName: "",
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mf := &MetricFamily{Name: &tt.metricName}
			result := NoFilter(mf)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStaticMetricFilter(t *testing.T) {
	tests := []struct {
		name        string
		filterNames []string
		metricName  string
		expectMatch bool
	}{
		{
			name:        "matches single metric",
			filterNames: []string{"test_metric"},
			metricName:  "test_metric",
			expectMatch: true,
		},
		{
			name:        "does not match different metric",
			filterNames: []string{"test_metric"},
			metricName:  "other_metric",
			expectMatch: false,
		},
		{
			name:        "matches one of multiple metrics",
			filterNames: []string{"metric_a", "metric_b", "metric_c"},
			metricName:  "metric_b",
			expectMatch: true,
		},
		{
			name:        "does not match when not in list",
			filterNames: []string{"metric_a", "metric_b"},
			metricName:  "metric_c",
			expectMatch: false,
		},
		{
			name:        "empty filter list matches nothing",
			filterNames: []string{},
			metricName:  "any_metric",
			expectMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := StaticMetricFilter(tt.filterNames...)
			mf := &MetricFamily{Name: &tt.metricName}
			result := filter(mf)
			assert.Equal(t, tt.expectMatch, result)
		})
	}
}

func TestRegexMetricFilter(t *testing.T) {
	tests := []struct {
		name        string
		regexes     []string
		metricName  string
		expectMatch bool
	}{
		{
			name:        "matches simple pattern",
			regexes:     []string{"^test_.*"},
			metricName:  "test_metric",
			expectMatch: true,
		},
		{
			name:        "does not match when pattern differs",
			regexes:     []string{"^test_.*"},
			metricName:  "prod_metric",
			expectMatch: false,
		},
		{
			name:        "matches one of multiple patterns",
			regexes:     []string{"^test_.*", "^prod_.*", ".*_gauge$"},
			metricName:  "prod_counter",
			expectMatch: true,
		},
		{
			name:        "matches suffix pattern",
			regexes:     []string{".*_counter$"},
			metricName:  "requests_counter",
			expectMatch: true,
		},
		{
			name:        "matches middle pattern",
			regexes:     []string{".*_http_.*"},
			metricName:  "server_http_requests",
			expectMatch: true,
		},
		{
			name:        "does not match any pattern",
			regexes:     []string{"^test_.*", ".*_counter$"},
			metricName:  "prod_gauge",
			expectMatch: false,
		},
		{
			name:        "empty regex list matches nothing",
			regexes:     []string{},
			metricName:  "any_metric",
			expectMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compiledRegexes := make([]regexp.Regexp, len(tt.regexes))
			for i, r := range tt.regexes {
				compiledRegexes[i] = *regexp.MustCompile(r)
			}
			filter := RegexMetricFilter(compiledRegexes...)
			mf := &MetricFamily{Name: &tt.metricName}
			result := filter(mf)
			assert.Equal(t, tt.expectMatch, result)
		})
	}
}

func TestBatchMetricFilter(t *testing.T) {
	tests := []struct {
		name        string
		filters     []MetricFilter
		metricName  string
		expectMatch bool
	}{
		{
			name: "matches when first filter matches",
			filters: []MetricFilter{
				StaticMetricFilter("test_metric"),
				StaticMetricFilter("other_metric"),
			},
			metricName:  "test_metric",
			expectMatch: true,
		},
		{
			name: "matches when second filter matches",
			filters: []MetricFilter{
				StaticMetricFilter("other_metric"),
				StaticMetricFilter("test_metric"),
			},
			metricName:  "test_metric",
			expectMatch: true,
		},
		{
			name: "matches when any filter matches",
			filters: []MetricFilter{
				StaticMetricFilter("metric_a"),
				RegexMetricFilter(*regexp.MustCompile("^test_.*")),
				StaticMetricFilter("metric_c"),
			},
			metricName:  "test_counter",
			expectMatch: true,
		},
		{
			name: "does not match when no filter matches",
			filters: []MetricFilter{
				StaticMetricFilter("metric_a"),
				StaticMetricFilter("metric_b"),
			},
			metricName:  "metric_c",
			expectMatch: false,
		},
		{
			name: "combines static and regex filters",
			filters: []MetricFilter{
				StaticMetricFilter("exact_match"),
				RegexMetricFilter(*regexp.MustCompile(".*_counter$")),
			},
			metricName:  "requests_counter",
			expectMatch: true,
		},
		{
			name:        "empty filter list matches nothing",
			filters:     []MetricFilter{},
			metricName:  "any_metric",
			expectMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := BatchMetricFilter(tt.filters...)
			mf := &MetricFamily{Name: &tt.metricName}
			result := filter(mf)
			assert.Equal(t, tt.expectMatch, result)
		})
	}
}
