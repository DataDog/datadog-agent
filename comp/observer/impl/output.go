// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// ObserverOutput is the top-level JSON structure produced by headless mode.
// The Go field uses ObserverCorrelation (internal domain type) while the JSON
// key is "anomaly_periods" — the consumer-facing name that describes what each
// entry represents: a time period during which correlated anomalies were active.
type ObserverOutput struct {
	Metadata       ObserverMetadata      `json:"metadata"`
	AnomalyPeriods []ObserverCorrelation `json:"anomaly_periods"`
}

// ObserverMetadata describes the scenario and pipeline configuration.
type ObserverMetadata struct {
	Scenario            string   `json:"scenario"`
	TimelineStart       int64    `json:"timeline_start"`
	TimelineEnd         int64    `json:"timeline_end"`
	DetectorsEnabled    []string `json:"detectors_enabled"`
	CorrelatorsEnabled  []string `json:"correlators_enabled"`
	TotalAnomalyPeriods int      `json:"total_anomaly_periods"`
}

// ObserverCorrelation is one correlation cluster.
// Always includes the time span (pattern, period_start, period_end).
// Verbose mode adds title, member_series, and nested anomalies.
type ObserverCorrelation struct {
	Pattern      string            `json:"pattern"`
	PeriodStart  int64             `json:"period_start"`
	PeriodEnd    int64             `json:"period_end"`
	Title        string            `json:"title,omitempty"`
	MemberSeries []string          `json:"member_series,omitempty"`
	Anomalies    []ObserverAnomaly `json:"anomalies,omitempty"`
}

// ObserverAnomaly is a single anomaly nested inside a correlation (verbose only).
type ObserverAnomaly struct {
	Timestamp      int64  `json:"timestamp"`
	Source         string `json:"source"`
	SourceSeriesID string `json:"source_series_id"`
	Detector       string `json:"detector"`
}

// WriteObserverOutput collects correlations and metadata from the TestBench
// and writes a structured JSON results file.
// When verbose is true, correlations include title, member series, and nested anomalies.
// When verbose is false, correlations include only the time span (pattern, period_start, period_end).
func (tb *TestBench) WriteObserverOutput(path string, verbose bool) error {
	tb.mu.RLock()
	correlations := tb.engine.StateView().CorrelationHistory()

	scenario := tb.loadedScenario
	timelineStart, timelineEnd, hasBounds := tb.engine.Storage().TimeBounds()

	// Collect enabled detector and correlator names
	var detectorNames []string
	var correlatorNames []string
	for name, ci := range tb.components {
		if !ci.enabled {
			continue
		}
		switch ci.entry.kind {
		case componentDetector:
			detectorNames = append(detectorNames, name)
		case componentCorrelator:
			correlatorNames = append(correlatorNames, name)
		}
	}
	tb.mu.RUnlock()

	sort.Strings(detectorNames)
	sort.Strings(correlatorNames)

	if !hasBounds {
		timelineStart = 0
		timelineEnd = 0
	}

	// Build output correlations
	outCorrelations := make([]ObserverCorrelation, len(correlations))
	for i, corr := range correlations {
		oc := ObserverCorrelation{
			Pattern:     corr.Pattern,
			PeriodStart: corr.FirstSeen,
			PeriodEnd:   corr.LastUpdated,
		}

		if verbose {
			oc.Title = corr.Title
			oc.MemberSeries = make([]string, len(corr.MemberSeriesIDs))
			for j, sid := range corr.MemberSeriesIDs {
				oc.MemberSeries[j] = string(sid)
			}
			oc.Anomalies = make([]ObserverAnomaly, len(corr.Anomalies))
			for j, a := range corr.Anomalies {
				oc.Anomalies[j] = ObserverAnomaly{
					Timestamp:      a.Timestamp,
					Source:         string(a.Source),
					SourceSeriesID: string(a.SourceSeriesID),
					Detector:       a.DetectorName,
				}
			}
		}

		outCorrelations[i] = oc
	}

	output := ObserverOutput{
		Metadata: ObserverMetadata{
			Scenario:            scenario,
			TimelineStart:       timelineStart,
			TimelineEnd:         timelineEnd,
			DetectorsEnabled:    detectorNames,
			CorrelatorsEnabled:  correlatorNames,
			TotalAnomalyPeriods: len(outCorrelations),
		},
		AnomalyPeriods: outCorrelations,
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling observer output: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing observer output to %s: %w", path, err)
	}

	return nil
}
