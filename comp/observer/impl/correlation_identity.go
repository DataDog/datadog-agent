// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"sort"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

func sortedUniqueMetricNames(anomalies []observer.AnomalyOutput) []observer.MetricName {
	seen := make(map[observer.MetricName]struct{})
	for _, a := range anomalies {
		if a.Source == "" {
			continue
		}
		seen[a.Source] = struct{}{}
	}
	names := make([]observer.MetricName, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	return names
}

func sortedUniqueSeriesIDs(anomalies []observer.AnomalyOutput) []observer.SeriesID {
	seen := make(map[observer.SeriesID]struct{})
	for _, a := range anomalies {
		if a.SourceSeriesID == "" {
			continue
		}
		seen[a.SourceSeriesID] = struct{}{}
	}
	ids := make([]observer.SeriesID, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
