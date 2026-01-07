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
	seenCorrelations map[string]string // pattern -> title for correlations we've reported
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

// Report checks correlation state and prints changes.
// It prints "[observer] NEW: {title}" when a correlation first appears
// and "[observer] CLEARED: {title}" when a correlation disappears.
func (r *StdoutReporter) Report(report observer.ReportOutput) {
	// If we have correlation state configured, check for changes
	if r.correlationState != nil {
		r.reportCorrelationChanges()
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
	for pattern, title := range currentlyActive {
		if _, seen := r.seenCorrelations[pattern]; !seen {
			fmt.Printf("[observer] NEW: %s\n", title)
			r.seenCorrelations[pattern] = title
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
