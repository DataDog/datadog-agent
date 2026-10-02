// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"sort"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// sortedUniqueMembers extracts unique SeriesDescriptors from anomalies' Source
// fields, deduplicating by Key() and sorting by Key() for deterministic output.
func sortedUniqueMembers(anomalies []observer.Anomaly) []observer.SeriesDescriptor {
	seen := make(map[string]observer.SeriesDescriptor)
	for _, a := range anomalies {
		key := a.Source.Key()
		if _, ok := seen[key]; !ok {
			seen[key] = a.Source
		}
	}
	// Reuse the keys already computed for deduplication. Building them inside
	// the comparison repeats tag traversal and string allocation O(n log n) times.
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	members := make([]observer.SeriesDescriptor, 0, len(seen))
	for _, key := range keys {
		members = append(members, seen[key])
	}
	return members
}
