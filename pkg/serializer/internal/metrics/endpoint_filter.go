// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/metrics/metricname"
)

// EndpointMetricFilter admits metric names matching an endpoint allowlist.
type EndpointMetricFilter struct {
	matcher metricname.Matcher
}

// Len returns the number of stored matcher entries.
func (f *EndpointMetricFilter) Len() int {
	return f.matcher.Len()
}

// Filter implements Filter.
func (f *EndpointMetricFilter) Filter(metric Filterable) bool {
	return f.matcher.Test(metric.GetName())
}

// CompileEndpointMetricFilters groups equivalent policies to share an encoder.
// Matching normalizes incoming names and preserves configured names and prefixes.
func CompileEndpointMetricFilters(routing configutils.MetricsEndpointRouting) map[string]*EndpointMetricFilter {
	filters := make(map[string]*EndpointMetricFilter)
	groups := make(map[string]*EndpointMetricFilter)
	for _, policies := range []map[string][]string{routing.MetricAllowlist, routing.MetricPrefixAllowlist} {
		for endpoint := range policies {
			if _, exists := filters[endpoint]; exists {
				continue
			}
			prefixes := routing.MetricPrefixAllowlist[endpoint]
			rules := make([]metricname.PrefixRule, 0, len(prefixes))
			for _, prefix := range prefixes {
				rules = append(rules, metricname.PrefixRule{Prefix: prefix})
			}
			matcher, _ := metricname.NewMatcherWithPrefixRules(routing.MetricAllowlist[endpoint], false, rules)
			key := matcher.Key()
			filter, exists := groups[key]
			if !exists {
				filter = &EndpointMetricFilter{matcher: matcher}
				groups[key] = filter
			}
			filters[endpoint] = filter
		}
	}
	return filters
}
