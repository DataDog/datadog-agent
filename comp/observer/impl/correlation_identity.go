// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"sort"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// sortedUniqueMembers extracts unique SeriesDescriptors from anomalies' Source
// fields, deduplicating by Key() and sorting by String() for deterministic output.
func sortedUniqueMembers(anomalies []observer.Anomaly) []observer.SeriesDescriptor {
	seen := make(map[string]observer.SeriesDescriptor)
	for _, a := range anomalies {
		key := a.Source.Key()
		if _, ok := seen[key]; !ok {
			seen[key] = a.Source
		}
	}
	members := make([]observer.SeriesDescriptor, 0, len(seen))
	for _, sd := range seen {
		members = append(members, sd)
	}
	sort.Slice(members, func(i, j int) bool { return members[i].Key() < members[j].Key() })
	return members
}
