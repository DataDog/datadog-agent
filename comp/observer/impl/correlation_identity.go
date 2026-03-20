// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"sort"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

func sortedUniqueMetricNames(anomalies []observer.Anomaly) []observer.MetricName {
	seen := make(map[observer.MetricName]struct{})
	for _, a := range anomalies {
		display := observer.MetricName(a.Source.String())
		if display == "" {
			continue
		}
		seen[display] = struct{}{}
	}
	names := make([]observer.MetricName, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	return names
}

func sortedUniqueRefs(anomalies []observer.Anomaly) []observer.SeriesRef {
	seen := make(map[observer.SeriesRef]struct{})
	for _, a := range anomalies {
		if a.SourceView.Ref == observer.NoSeriesRef {
			continue
		}
		seen[a.SourceView.Ref] = struct{}{}
	}
	refs := make([]observer.SeriesRef, 0, len(seen))
	for ref := range seen {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i] < refs[j] })
	return refs
}

func uniqueViewStrings(anomalies []observer.Anomaly) []string {
	seen := make(map[string]struct{})
	for _, a := range anomalies {
		s := a.SourceView.String()
		if s == "" {
			continue
		}
		seen[s] = struct{}{}
	}
	views := make([]string, 0, len(seen))
	for v := range seen {
		views = append(views, v)
	}
	sort.Strings(views)
	return views
}
