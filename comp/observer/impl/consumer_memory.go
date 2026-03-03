// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"strings"

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

// Process adds an anomaly to the pending list.
func (p *PassthroughCorrelator) Process(anomaly observer.Anomaly) {
	p.anomalies = append(p.anomalies, anomaly)
}

// Flush converts accumulated anomalies to reports and clears the list.
func (p *PassthroughCorrelator) Flush() []observer.ReportOutput {
	if len(p.anomalies) == 0 {
		return nil
	}

	reports := make([]observer.ReportOutput, len(p.anomalies))
	for i, a := range p.anomalies {
		reports[i] = observer.ReportOutput{
			Title: a.Title,
			Body:  a.Description,
			Metadata: map[string]string{
				"tags": strings.Join(a.Tags, ","),
			},
		}
	}

	p.anomalies = nil
	return reports
}

// GetPending returns pending anomalies (for testing).
func (p *PassthroughCorrelator) GetPending() []observer.Anomaly {
	return p.anomalies
}

// StdoutReporter prints reports to stdout.
// It tracks correlation state changes and only prints when correlations appear or disappear.
type StdoutReporter struct {
	correlationState observer.CorrelationState
	rawAnomalyState  observer.RawAnomalyState
	seenCorrelations map[string]string // pattern -> title for correlations we've reported
	seenRawAnomalies map[string]bool   // source|detector -> whether we've reported this raw anomaly
}

// Name returns the reporter name.
func (r *StdoutReporter) Name() string {
	return "stdout_reporter"
}

// SetCorrelationState sets the correlation state source for the reporter.
func (r *StdoutReporter) SetCorrelationState(state observer.CorrelationState) {
	r.correlationState = state
	r.seenCorrelations = make(map[string]string)
}

// SetRawAnomalyState sets the raw anomaly state source for the reporter.
func (r *StdoutReporter) SetRawAnomalyState(state observer.RawAnomalyState) {
	r.rawAnomalyState = state
	r.seenRawAnomalies = make(map[string]bool)
}

// Report checks correlation state and prints changes.
// It prints "[observer] NEW: {title}" when a correlation first appears
// and "[observer] CLEARED: {title}" when a correlation disappears.
func (r *StdoutReporter) Report(report observer.ReportOutput) {
	// Report raw anomalies first (with detector identification)
	if r.rawAnomalyState != nil {
		r.reportRawAnomalyChanges()
	}
	// If we have correlation state configured, check for changes
	if r.correlationState != nil {
		r.reportCorrelationChanges()
	}
}

// reportRawAnomalyChanges prints new raw anomalies with their detector source.
func (r *StdoutReporter) reportRawAnomalyChanges() {
	if r.seenRawAnomalies == nil {
		r.seenRawAnomalies = make(map[string]bool)
	}

	rawAnomalies := r.rawAnomalyState.RawAnomalies()

	for _, anomaly := range rawAnomalies {
		key := string(anomaly.Source) + "|" + anomaly.DetectorName
		if !r.seenRawAnomalies[key] {
			fmt.Printf("[observer] [%s] ANOMALY: %s\n", anomaly.DetectorName, anomaly.Source)
			fmt.Printf("           %s\n", anomaly.Description)
			r.seenRawAnomalies[key] = true
		}
	}
}

// reportCorrelationChanges checks for new and cleared correlations.
func (r *StdoutReporter) reportCorrelationChanges() {
	if r.seenCorrelations == nil {
		r.seenCorrelations = make(map[string]string)
	}

	// Get current active correlations
	activeCorrelations := r.correlationState.ActiveCorrelations()

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

// PrintFinalState prints the current state of all correlations and raw anomalies.
// Call this at the end of a demo to see final cluster contents.
func (r *StdoutReporter) PrintFinalState() {
	// Print raw anomaly summary by detector
	if r.rawAnomalyState != nil {
		rawAnomalies := r.rawAnomalyState.RawAnomalies()
		if len(rawAnomalies) > 0 {
			byDetector := make(map[string][]observer.Anomaly)
			for _, a := range rawAnomalies {
				byDetector[a.DetectorName] = append(byDetector[a.DetectorName], a)
			}

			fmt.Println("[observer] Raw Anomaly Summary:")
			for detector, anomalies := range byDetector {
				sources := make(map[observer.MetricName]bool)
				for _, a := range anomalies {
					sources[a.Source] = true
				}
				fmt.Printf("  [%s]: %d anomalies across %d metrics\n", detector, len(anomalies), len(sources))
				for _, a := range anomalies {
					fmt.Printf("    - %s\n", a.Description)
				}
			}
		}
	}

	// Print correlation summary
	if r.correlationState == nil {
		return
	}
	activeCorrelations := r.correlationState.ActiveCorrelations()
	if len(activeCorrelations) == 0 {
		fmt.Println("[observer] Final state: no active correlations")
		return
	}
	fmt.Println("[observer] Correlation Summary:")
	for _, ac := range activeCorrelations {
		fmt.Printf("  Cluster: %d anomalies\n", len(ac.Anomalies))
		for _, anomaly := range ac.Anomalies {
			fmt.Printf("    - %s\n", anomaly.Description)
		}
	}
}
