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

// PassthroughProcessor is a simple anomaly processor that converts each anomaly to a report.
// It serves as an example implementation and for testing.
type PassthroughProcessor struct {
	anomalies []observer.AnomalyOutput
}

// Name returns the processor name.
func (p *PassthroughProcessor) Name() string {
	return "passthrough_processor"
}

// Process adds an anomaly to the pending list.
func (p *PassthroughProcessor) Process(anomaly observer.AnomalyOutput) {
	p.anomalies = append(p.anomalies, anomaly)
}

// Flush converts accumulated anomalies to reports and clears the list.
func (p *PassthroughProcessor) Flush() []observer.ReportOutput {
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
func (p *PassthroughProcessor) GetPending() []observer.AnomalyOutput {
	return p.anomalies
}

// StdoutReporter prints reports to stdout.
// It tracks correlation state changes and only prints when correlations appear or disappear.
type StdoutReporter struct {
	correlationState observer.CorrelationState
	rawAnomalyState  observer.RawAnomalyState
	seenCorrelations map[string]string // pattern -> title for correlations we've reported
	seenRawAnomalies map[string]bool   // source|analyzer -> whether we've reported this raw anomaly
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
func (r *StdoutReporter) Report(_ observer.ReportOutput) {
	// Report raw anomalies first (with analyzer identification)
	if r.rawAnomalyState != nil {
		r.reportRawAnomalyChanges()
	}
	// If we have correlation state configured, check for changes
	if r.correlationState != nil {
		r.reportCorrelationChanges()
	}
}

// reportRawAnomalyChanges prints new raw anomalies with their analyzer source.
func (r *StdoutReporter) reportRawAnomalyChanges() {
	if r.seenRawAnomalies == nil {
		r.seenRawAnomalies = make(map[string]bool)
	}

	rawAnomalies := r.rawAnomalyState.RawAnomalies()

	for _, anomaly := range rawAnomalies {
		key := anomaly.Source + "|" + anomaly.AnalyzerName
		if !r.seenRawAnomalies[key] {
			fmt.Printf("[observer] [%s] ANOMALY: %s\n", anomaly.AnalyzerName, anomaly.Source)
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
	// Print raw anomaly summary by analyzer
	if r.rawAnomalyState != nil {
		rawAnomalies := r.rawAnomalyState.RawAnomalies()
		if len(rawAnomalies) > 0 {
			byAnalyzer := make(map[string][]observer.AnomalyOutput)
			for _, a := range rawAnomalies {
				byAnalyzer[a.AnalyzerName] = append(byAnalyzer[a.AnalyzerName], a)
			}

			fmt.Println("[observer] Raw Anomaly Summary:")
			for analyzer, anomalies := range byAnalyzer {
				sources := make(map[string]bool)
				for _, a := range anomalies {
					sources[a.Source] = true
				}
				fmt.Printf("  [%s]: %d anomalies across %d metrics\n", analyzer, len(anomalies), len(sources))
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
