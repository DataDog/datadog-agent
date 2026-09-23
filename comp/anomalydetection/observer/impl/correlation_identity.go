// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"

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
			if a.SourceRef == nil {
				members.handles = append(members.handles, nil)
			} else {
				handle := *a.SourceRef
				members.handles = append(members.handles, &handle)
			}
		}
	}
	return members
}
