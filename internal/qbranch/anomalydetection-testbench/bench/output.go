// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	reporterimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/impl"
)

// ObserverOutput is the top-level JSON structure produced by headless mode.
type ObserverOutput struct {
	Metadata       ObserverMetadata      `json:"metadata"`
	AnomalyPeriods []ObserverCorrelation `json:"anomaly_periods"`
	// RawDetectorAnomalies maps detector name → sorted list of anomaly timestamps
	// (unix seconds). Populated when verbose is true, from raw engine anomalies
	// before correlation. Useful for scoring detectors that bypass the correlator.
	RawDetectorAnomalies map[string][]int64 `json:"raw_detector_anomalies,omitempty"`
}

// ObserverMetadata describes the scenario and pipeline configuration.
type ObserverMetadata struct {
	Scenario            string   `json:"scenario"`
	TimelineStart       int64    `json:"timeline_start"`
	TimelineEnd         int64    `json:"timeline_end"`
	DetectorsEnabled    []string `json:"detectors_enabled"`
	CorrelatorsEnabled  []string `json:"correlators_enabled"`
	TotalAnomalyPeriods int      `json:"total_anomaly_periods"`
	// ComponentConfigs holds the active configuration of every component.
	ComponentConfigs map[string]map[string]any `json:"component_configs,omitempty"`
	Stats            *ReplayStats              `json:"stats,omitempty"`
}

// ObserverCorrelation is one correlation cluster.
type ObserverCorrelation struct {
	Pattern      string            `json:"pattern"`
	PeriodStart  int64             `json:"period_start"`
	PeriodEnd    int64             `json:"period_end"`
	Title        string            `json:"title,omitempty"`
	Message      string            `json:"message,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
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

// WriteObserverOutput collects correlations and metadata from the Bench
// and writes a structured JSON results file.
// When verbose is true, correlations include title, member series, and nested anomalies.
// When verbose is false, correlations include only the time span (pattern, period_start, period_end).
func (tb *Bench) WriteObserverOutput(path string, verbose bool) error {
	tb.mu.RLock()
	sv := tb.debug.StateView()
	correlations := sv.CorrelationHistory()

	scenario := tb.loadedScenario
	timelineStart, timelineEnd, hasBounds := sv.ScenarioBounds()

	// Collect enabled detector / correlator names from StateView.
	var detectorNames []string
	var correlatorNames []string
	for _, d := range sv.ListDetectors() {
		if d.Enabled {
			detectorNames = append(detectorNames, d.Name)
		}
	}
	for _, c := range sv.ListCorrelators() {
		if c.Enabled {
			correlatorNames = append(correlatorNames, c.Name)
		}
	}

	// Build component configs from catalog + settings.
	entries := tb.debug.CatalogEntries()
	componentConfigs := make(map[string]map[string]any, len(entries))
	for _, e := range entries {
		enabled := e.DefaultEnabled
		if v, ok := tb.settings.Enabled[e.Name]; ok {
			enabled = v
		}
		componentConfigs[e.Name] = map[string]any{"enabled": enabled}
	}

	replayStats := tb.replayStats
	tb.mu.RUnlock()

	sort.Strings(detectorNames)
	sort.Strings(correlatorNames)

	if !hasBounds {
		timelineStart = 0
		timelineEnd = 0
	}

	outCorrelations := make([]ObserverCorrelation, len(correlations))
	for i, corr := range correlations {
		oc := ObserverCorrelation{
			Pattern:     corr.Pattern,
			PeriodStart: corr.FirstSeen,
			PeriodEnd:   corr.LastUpdated,
		}

		if verbose {
			oc.Title = corr.Title
			oc.Message = reporterimpl.BuildChangeMessage(corr, nil)
			oc.Tags = []string{"source:agent-q-branch-observer", "pattern:" + corr.Pattern}
			oc.MemberSeries = make([]string, len(corr.Members))
			for j, m := range corr.Members {
				oc.MemberSeries[j] = m.DisplayName()
			}
			oc.Anomalies = make([]ObserverAnomaly, len(corr.Anomalies))
			for j, a := range corr.Anomalies {
				sourceID := a.Source.Key()
				if a.SourceRef != nil {
					sourceID = a.SourceRef.CompactID()
				}
				oc.Anomalies[j] = ObserverAnomaly{
					Timestamp:      a.Timestamp,
					Source:         a.Source.String(),
					SourceSeriesID: sourceID,
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
			ComponentConfigs:    componentConfigs,
			Stats:               replayStats,
		},
		AnomalyPeriods: outCorrelations,
	}

	if verbose {
		byDetector := sv.AnomaliesByDetector()
		if len(byDetector) > 0 {
			rawMap := make(map[string][]int64, len(byDetector))
			for detName, anomalies := range byDetector {
				timestamps := make([]int64, len(anomalies))
				for i, a := range anomalies {
					timestamps[i] = a.Timestamp
				}
				sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })
				rawMap[detName] = timestamps
			}
			output.RawDetectorAnomalies = rawMap
		}
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
