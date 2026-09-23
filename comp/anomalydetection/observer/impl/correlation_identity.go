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
// fields, structurally deduplicating them and sorting by their legacy serialized
// representation for deterministic output.
func sortedUniqueMembers(anomalies []observer.Anomaly) []observer.SeriesDescriptor {
	members := make([]observer.SeriesDescriptor, 0, len(anomalies))
	for _, a := range anomalies {
		members = append(members, a.Source)
	}
	sort.SliceStable(members, func(i, j int) bool {
		return compareSeriesDescriptors(members[i], members[j]) < 0
	})

	unique := members[:0]
	for _, member := range members {
		if len(unique) == 0 || !seriesDescriptorsEqual(unique[len(unique)-1], member) {
			unique = append(unique, member)
		}
	}
	return unique
}
