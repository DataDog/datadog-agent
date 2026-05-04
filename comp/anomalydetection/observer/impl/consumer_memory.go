// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// PassthroughCorrelator is a simple correlator that converts each anomaly to a report.
// It serves as an example implementation and for testing.
type PassthroughCorrelator struct {
	anomalies []observer.Anomaly
}

// Name returns the correlator name.
func (p *PassthroughCorrelator) Name() string {
	return "passthrough_correlator"
}

// ProcessAnomaly adds an anomaly to the pending list.
func (p *PassthroughCorrelator) ProcessAnomaly(a observer.Anomaly) {
	p.anomalies = append(p.anomalies, a)
}

// Advance is a no-op for the passthrough correlator (no time-based eviction).
func (p *PassthroughCorrelator) Advance(_ int64) {}

// ActiveCorrelations returns empty (passthrough does not produce correlations).
func (p *PassthroughCorrelator) ActiveCorrelations() []observer.ActiveCorrelation {
	return nil
}

// Reset clears accumulated anomalies.
func (p *PassthroughCorrelator) Reset() {
	p.anomalies = nil
}

// GetPending returns pending anomalies (for testing).
func (p *PassthroughCorrelator) GetPending() []observer.Anomaly {
	return p.anomalies
}
