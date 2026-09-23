// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"sort"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

type correlationMembers struct {
	descriptors []observer.SeriesDescriptor
	handles     []*observer.QueryHandle
}

// uniqueMembers extracts one display descriptor for each anomaly source identity.
// Storage-backed anomalies use QueryHandle; ref-less anomalies use the fallback
// identity in anomalySourceIdentityFor.
func uniqueMembers(anomalies []observer.Anomaly) correlationMembers {
	seen := make(map[anomalySourceIdentity]observer.SeriesDescriptor, len(anomalies))
	members := correlationMembers{
		descriptors: make([]observer.SeriesDescriptor, 0, len(anomalies)),
		handles:     make([]*observer.QueryHandle, 0, len(anomalies)),
	}
	for _, a := range anomalies {
		identity := anomalySourceIdentityFor(a)
		if _, exists := seen[identity]; !exists {
			seen[identity] = a.Source
			members.descriptors = append(members.descriptors, a.Source)
			members.handles = append(members.handles, a.SourceRef)
		}
	}
	return members
}

// sortedForDisplay returns members in the legacy descriptor order. It is called
// only while materializing correlation or debug output, never while ingesting
// anomalies or deduplicating storage-backed identity.
func (m correlationMembers) sortedForDisplay() (correlationMembers, []string) {
	if len(m.descriptors) == 0 {
		return m, nil
	}

	indices := make([]int, len(m.descriptors))
	formatted := make([]string, len(m.descriptors))
	for i := range indices {
		indices[i] = i
		formatted[i] = formatSeriesDescriptor(m.descriptors[i])
	}
	sort.Slice(indices, func(i, j int) bool {
		return formatted[indices[i]] < formatted[indices[j]]
	})

	sorted := correlationMembers{
		descriptors: make([]observer.SeriesDescriptor, len(m.descriptors)),
		handles:     make([]*observer.QueryHandle, len(m.handles)),
	}
	sortedFormatted := make([]string, len(formatted))
	for i, index := range indices {
		sorted.descriptors[i] = m.descriptors[index]
		sorted.handles[i] = m.handles[index]
		sortedFormatted[i] = formatted[index]
	}
	return sorted, sortedFormatted
}
