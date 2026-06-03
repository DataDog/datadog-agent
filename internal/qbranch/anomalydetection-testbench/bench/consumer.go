// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"fmt"

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

// StdoutReporter prints reports to stdout.
// It tracks correlation state changes and only prints when correlations appear or disappear.
type StdoutReporter struct {
	seenCorrelations map[string]string
	seenRawAnomalies map[string]bool
	lastCorrelations []observer.ActiveCorrelation
}

// Name returns the reporter name.
func (r *StdoutReporter) Name() string {
	return "stdout_reporter"
}

// Report receives a ReportOutput with anomalies and correlations.
func (r *StdoutReporter) Report(report observer.ReportOutput) {
	r.reportNewAnomalies(report.NewAnomalies)
	r.reportCorrelationChanges(report.ActiveCorrelations)
	r.lastCorrelations = report.ActiveCorrelations
}

func (r *StdoutReporter) reportNewAnomalies(anomalies []observer.Anomaly) {
	if r.seenRawAnomalies == nil {
		r.seenRawAnomalies = make(map[string]bool)
	}

	for _, anomaly := range anomalies {
		key := anomaly.Source.String() + "|" + anomaly.DetectorName
		if !r.seenRawAnomalies[key] {
			fmt.Printf("[observer] [%s] ANOMALY: %s\n", anomaly.DetectorName, anomaly.Source.String())
			fmt.Printf("           %s\n", anomaly.Description)
			r.seenRawAnomalies[key] = true
		}
	}
}

func (r *StdoutReporter) reportCorrelationChanges(activeCorrelations []observer.ActiveCorrelation) {
	if r.seenCorrelations == nil {
		r.seenCorrelations = make(map[string]string)
	}

	currentlyActive := make(map[string]string)
	for _, ac := range activeCorrelations {
		currentlyActive[ac.Pattern] = ac.Title
	}

	for _, ac := range activeCorrelations {
		if _, seen := r.seenCorrelations[ac.Pattern]; !seen {
			fmt.Printf("[observer] NEW: %s\n", ac.Title)
			for _, anomaly := range ac.Anomalies {
				fmt.Printf("  - %s\n", anomaly.Description)
			}
			r.seenCorrelations[ac.Pattern] = ac.Title
		}
	}

	for pattern, title := range r.seenCorrelations {
		if _, ok := currentlyActive[pattern]; !ok {
			fmt.Printf("[observer] CLEARED: %s\n", title)
			delete(r.seenCorrelations, pattern)
		}
	}
}

// PrintFinalState prints the current state of all correlations.
func (r *StdoutReporter) PrintFinalState() {
	if len(r.lastCorrelations) == 0 {
		fmt.Println("[observer] Final state: no active correlations")
		return
	}
	fmt.Println("[observer] Correlation Summary:")
	for _, ac := range r.lastCorrelations {
		fmt.Printf("  Cluster: %d anomalies\n", len(ac.Anomalies))
		for _, anomaly := range ac.Anomalies {
			fmt.Printf("    - %s\n", anomaly.Description)
		}
	}
}
