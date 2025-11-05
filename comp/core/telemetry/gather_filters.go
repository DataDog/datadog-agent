// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build !serverless

package telemetry

import (
	"regexp"
	"slices"
)

// NoFilter returns a MetricFilter that includes all metrics
// This is not recommended since it will heavily impact costs
func NoFilter(*MetricFamily) bool {
	return true
}

// StaticMetricFilter filters metrics based on their name
// It returns true if the metric name is in the list, false otherwise
func StaticMetricFilter(metricNames ...string) MetricFilter {
	return func(mf *MetricFamily) bool {
		return slices.Contains(metricNames, mf.GetName())
	}
}

// RegexMetricFilter filters metrics based on their name using regular expressions
// It returns true if the metric name matches at least one of the regular expressions, false otherwise
func RegexMetricFilter(regexes ...regexp.Regexp) MetricFilter {
	return func(mf *MetricFamily) bool {
		for _, regex := range regexes {
			if regex.MatchString(mf.GetName()) {
				return true
			}
		}
		return false
	}
}

// BatchMetricFilter combines multiple MetricFilters into a single MetricFilter
// It returns true if at least one of the filters return true, false otherwise
func BatchMetricFilter(filters ...MetricFilter) MetricFilter {
	return func(mf *MetricFamily) bool {
		for _, filter := range filters {
			if filter(mf) {
				return true
			}
		}
		return false
	}
}
