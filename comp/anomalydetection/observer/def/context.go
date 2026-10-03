// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observer

// ResolveAnomalyContext returns a copy enriched with the current series context.
// Raw anomalies can be retained and scored without materializing presentation data.
func ResolveAnomalyContext(a Anomaly, storage StorageReader) Anomaly {
	if a.Context != nil || a.SourceRef == nil || storage == nil {
		return a
	}
	if ctx, ok := storage.GetContext(a.SourceRef.Ref); ok {
		a.Context = &ctx
	}
	return a
}

// MaterializedAnomaly contains the resolved context and formatted detector text.
type MaterializedAnomaly struct {
	Anomaly     Anomaly
	Title       string
	Description string
}

// MaterializeAnomaly prepares an anomaly only when an output needs its context
// or detector text. It does not modify the input anomaly.
func MaterializeAnomaly(a Anomaly, storage StorageReader) MaterializedAnomaly {
	a = ResolveAnomalyContext(a, storage)
	title, description := FormatAnomaly(a)
	return MaterializedAnomaly{Anomaly: a, Title: title, Description: description}
}
