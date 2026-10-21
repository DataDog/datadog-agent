// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"encoding/json"
	"slices"
	"strings"

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
			prefixes := slices.Clone(routing.MetricPrefixAllowlist[endpoint])
			slices.Sort(prefixes)
			compacted := make([]string, 0, len(prefixes))
			for _, prefix := range prefixes {
				if len(compacted) == 0 || !strings.HasPrefix(prefix, compacted[len(compacted)-1]) {
					compacted = append(compacted, prefix)
				}
			}
			names := slices.Clone(routing.MetricAllowlist[endpoint])
			slices.Sort(names)
			names = slices.DeleteFunc(slices.Compact(names), func(name string) bool {
				i, found := slices.BinarySearch(compacted, name)
				return found || (i > 0 && strings.HasPrefix(name, compacted[i-1]))
			})
			// JSON avoids ambiguous group keys when entries contain separators.
			key, _ := json.Marshal([2][]string{append([]string{}, names...), compacted})
			filter, exists := groups[string(key)]
			if !exists {
				rules := make([]metricname.PrefixRule, 0, len(compacted))
				for _, prefix := range compacted {
					rules = append(rules, metricname.PrefixRule{Prefix: prefix})
				}
				matcher, _ := metricname.NewMatcherWithPrefixRules(names, false, rules)
				filter = &EndpointMetricFilter{matcher: matcher}
				groups[string(key)] = filter
			}
			filters[endpoint] = filter
		}
	}
	return filters
}
