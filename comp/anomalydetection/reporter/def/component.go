// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package reporter provides a component that formats and dispatches anomaly
// detection events to the Datadog backend or stdout.
package reporter

import observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"

// team: q-branch

// Component is the reporter component type.
type Component interface{}

// ReportOutput is the output model passed to reporters after each advance cycle.
// It carries enough data for reporters to act without reaching back into engine internals.
type ReportOutput struct {
	// AdvancedToSec is the data time the engine advanced to.
	AdvancedToSec int64
	// NewAnomalies are anomalies detected in this advance cycle.
	NewAnomalies []observerdef.Anomaly
	// ActiveCorrelations are the current correlation patterns across all correlators.
	ActiveCorrelations []observerdef.ActiveCorrelation
}

// Reporter receives reports and displays or delivers them.
type Reporter interface {
	// Name returns the reporter name for debugging.
	Name() string
	// Report delivers a report to its destination (stdout, file, webserver, etc).
	Report(report ReportOutput)
}

// StorageConsumer is an optional interface for reporters that need access to
// the engine's time-series storage (e.g. for rate annotations in event messages).
// The observer calls SetStorage after constructing the engine, injecting storage
// without creating a circular Fx dependency between reporter and observer.
type StorageConsumer interface {
	SetStorage(storage observerdef.StorageReader)
}
