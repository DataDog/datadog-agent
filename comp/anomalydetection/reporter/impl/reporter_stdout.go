// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package reporterimpl

import (
	"fmt"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

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
	r.reportNewAnomalies(report.NewAnomalies)
	r.reportCorrelationChanges(report.ActiveCorrelations)
	r.lastCorrelations = report.ActiveCorrelations
}

// reportNewAnomalies prints new anomalies from this advance cycle.
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

// reportCorrelationChanges checks for new and cleared correlations.
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
