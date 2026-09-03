// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"fmt"
	"sort"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// passthroughCorrelations converts raw anomalies into one evaluation period
// each. It lives in the testbench instead of the observer component catalog so
// this evaluation adapter can never run in the production agent pipeline.
func passthroughCorrelations(anomalies []observer.Anomaly) []observer.ActiveCorrelation {
	byDetector := make(map[string][]observer.Anomaly)
	for _, anomaly := range anomalies {
		byDetector[anomaly.DetectorName] = append(byDetector[anomaly.DetectorName], anomaly)
	}

	detectorNames := make([]string, 0, len(byDetector))
	for name := range byDetector {
		detectorNames = append(detectorNames, name)
	}
	sort.Strings(detectorNames)

	var correlations []observer.ActiveCorrelation
	for _, detectorName := range detectorNames {
		detected := append([]observer.Anomaly(nil), byDetector[detectorName]...)
		sort.SliceStable(detected, func(i, j int) bool {
			return detected[i].Timestamp < detected[j].Timestamp
		})
		for i, anomaly := range detected {
			correlations = append(correlations, observer.ActiveCorrelation{
				Pattern:     fmt.Sprintf("passthrough_%s_%d", detectorName, i),
				Title:       fmt.Sprintf("Passthrough[%s]: %s", detectorName, anomaly.Source),
				Members:     []observer.SeriesDescriptor{anomaly.Source},
				Anomalies:   []observer.Anomaly{anomaly},
				FirstSeen:   anomaly.Timestamp,
				LastUpdated: anomaly.Timestamp,
			})
		}
	}
	return correlations
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
