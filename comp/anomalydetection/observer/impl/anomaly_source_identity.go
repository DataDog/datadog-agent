// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"

// anomalySourceIdentity is a comparable identity for an anomaly source.
// Storage-backed anomalies use their QueryHandle, which distinguishes aggregates
// without constructing a string. Anomalies without a storage-backed source use a
// temporary descriptor-key fallback until SeriesDescriptor.Key is removed.
type anomalySourceIdentity struct {
	handle    observerdef.QueryHandle
	fallback  string
	hasHandle bool
}

func anomalySourceIdentityFor(anomaly observerdef.Anomaly) anomalySourceIdentity {
	if anomaly.SourceRef != nil {
		return anomalySourceIdentity{
			handle:    *anomaly.SourceRef,
			hasHandle: true,
		}
	}
	return anomalySourceIdentity{fallback: anomaly.Source.Key()}
}
