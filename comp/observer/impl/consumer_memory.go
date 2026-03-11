// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
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
// All data comes through Report(ReportOutput) — no backdoor access to engine internals.
type StdoutReporter struct {
	seenCorrelations map[string]string // pattern -> title for correlations we've reported
	seenRawAnomalies map[string]bool   // source|detector -> whether we've reported this raw anomaly
	// lastCorrelations is cached from the most recent Report call for PrintFinalState.
	lastCorrelations []observer.ActiveCorrelation
}

// Name returns the reporter name.
func (r *StdoutReporter) Name() string {
	return "stdout_reporter"
}

// Report receives a ReportOutput with anomalies and correlations from the engine
// and prints changes. It prints new anomalies and tracks correlation state changes,
// printing "[observer] NEW: {title}" when a correlation first appears and
// "[observer] CLEARED: {title}" when a correlation disappears.
func (r *StdoutReporter) Report(report observer.ReportOutput) {
	// Report new anomalies (with detector identification)
	r.reportNewAnomalies(report.NewAnomalies)
	// Check for correlation changes
	r.reportCorrelationChanges(report.ActiveCorrelations)
	// Cache for PrintFinalState
	r.lastCorrelations = report.ActiveCorrelations
}

// reportNewAnomalies prints new anomalies from this advance cycle.
func (r *StdoutReporter) reportNewAnomalies(anomalies []observer.Anomaly) {
	if r.seenRawAnomalies == nil {
		r.seenRawAnomalies = make(map[string]bool)
	}

	for _, anomaly := range anomalies {
		key := string(anomaly.Source) + "|" + anomaly.DetectorName
		if !r.seenRawAnomalies[key] {
			fmt.Printf("[observer] [%s] ANOMALY: %s\n", anomaly.DetectorName, anomaly.Source)
			fmt.Printf("           %s\n", anomaly.Description)
			r.seenRawAnomalies[key] = true
		}
	}
}

// reportCorrelationChanges checks for new and cleared correlations.
func (r *StdoutReporter) reportCorrelationChanges(activeCorrelations []observer.ActiveCorrelation) {
	if r.seenCorrelations == nil {
		r.seenCorrelations = make(map[string]string)
	}

	// Build set of currently active pattern names
	currentlyActive := make(map[string]string) // pattern -> title
	for _, ac := range activeCorrelations {
		currentlyActive[ac.Pattern] = ac.Title
	}

	// Check for new correlations (in current but not in seen)
	for _, ac := range activeCorrelations {
		if _, seen := r.seenCorrelations[ac.Pattern]; !seen {
			fmt.Printf("[observer] NEW: %s\n", ac.Title)
			for _, anomaly := range ac.Anomalies {
				fmt.Printf("  - %s\n", anomaly.Description)
			}
			r.seenCorrelations[ac.Pattern] = ac.Title
		}
	}

	// Check for cleared correlations (in seen but not in current)
	for pattern, title := range r.seenCorrelations {
		if _, ok := currentlyActive[pattern]; !ok {
			fmt.Printf("[observer] CLEARED: %s\n", title)
			delete(r.seenCorrelations, pattern)
		}
	}
}

// PrintFinalState prints the current state of all correlations.
// Call this at the end of a demo to see final cluster contents.
// Uses the last correlations received via Report.
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
