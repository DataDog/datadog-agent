// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package reporter defines the reporter component contracts.
// Concrete reporters are provided through the `anomalydetection_reporters` Fx group.
package reporter

// team: q-branch

import observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"

// Component is the top-level marker interface required by the component linter.
// Concrete reporters register via the `anomalydetection_reporters` Fx group as
// Reporter values rather than through this interface.
type Component = any

// Reporter is the per-reporter contract delivered through the
// `anomalydetection_reporters` Fx group.
type Reporter interface {
	// Name returns the reporter name for identification.
	Name() string
	// Report is called by the observer after each detection cycle.
	Report(output ReportOutput)
}

// ReportOutput carries the data reporters receive after each detection cycle.
// It re-uses observerdef types directly so reporters can build rich messages
// (log-rate annotations, debug info, correlation members) without lossy conversion.
// reporter/def therefore depends on observer/def — observer/def owns the canonical schema.
type ReportOutput struct {
	// AdvancedToSec is the data time the engine advanced to.
	AdvancedToSec int64
	// NewAnomalies are anomalies detected in this advance cycle.
	NewAnomalies []observerdef.Anomaly
	// ActiveCorrelations are the patterns currently held in each correlator's
	// sliding window. A pattern leaves this set when it goes inactive
	// (eviction, timeout) and rejoins if it recurs.
	ActiveCorrelations []observerdef.ActiveCorrelation
	// CorrelationHistory is the accumulated set of every correlation pattern
	// the engine has detected during the current run, including ones whose
	// changepoint timestamps are already old enough to be evicted from
	// ActiveCorrelations (e.g. batch detector clusters). Reporters that want
	// to emit exactly once per pattern should drive emission from this set
	// and use ActiveCorrelations to decide when a pattern has gone inactive.
	CorrelationHistory []observerdef.ActiveCorrelation
}

// StorageConsumer is an optional interface for reporters that need access to
// the engine's time-series storage (e.g. for windowed log-rate annotations).
// The observer calls SetStorage exactly once after engine construction, before
// the first Report call.
type StorageConsumer interface {
	SetStorage(storage observerdef.StorageReader)
}
