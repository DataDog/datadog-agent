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

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	reporterimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/impl"
)

// ObserverOutput is the top-level JSON structure produced by headless mode.
type ObserverOutput struct {
	Metadata       ObserverMetadata       `json:"metadata"`
	AnomalyPeriods []ObserverCorrelation  `json:"anomaly_periods"`
	AnomalyEvents  []ObserverAnomalyEvent `json:"anomaly_events,omitempty"`
}

// ObserverAnomalyEvent is one scored anomaly event candidate (verbose only).
type ObserverAnomalyEvent struct {
	ID               string `json:"id"`
	Scope            string `json:"scope"`
	TriggerTimestamp int64  `json:"trigger_timestamp"`
	TriggerSource    string `json:"trigger_source"`
	TriggerDetector  string `json:"trigger_detector"`
	TriggerType      string `json:"trigger_type"`
	// Instant is the sliding-window noisy-OR score.
	Instant float64 `json:"instant"`
	// EWMA is the per-scope smoothed score.
	EWMA             float64 `json:"ewma"`
	PreviousEWMA     float64 `json:"previous_ewma"`
	Severity         string  `json:"severity"`
	PreviousSeverity string  `json:"previous_severity,omitempty"`
	SeverityChanged  bool    `json:"severity_changed"`
	Trend            string  `json:"trend"`
	SignalCount      int     `json:"signal_count"`
	EffectiveSignals int     `json:"effective_signals"`
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
// When verbose is true, correlations include title, member series, nested anomalies,
// and the full list of scored anomaly events.
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
	anomalyEvents := sv.AnomalyEvents()
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

	var outAnomalyEvents []ObserverAnomalyEvent
	if verbose && len(anomalyEvents) > 0 {
		outAnomalyEvents = make([]ObserverAnomalyEvent, 0, len(anomalyEvents))
		for _, ae := range anomalyEvents {
			t := ae.Anomaly
			sc := ae.Score
			trigType := "metric"
			if t.Type == observerdef.AnomalyTypeLog {
				trigType = "log"
			} else if t.Source.Namespace == "log_pattern_extractor" || t.Source.Namespace == "log_metrics_extractor" {
				trigType = "log"
			}
			outAnomalyEvents = append(outAnomalyEvents, ObserverAnomalyEvent{
				ID:               ae.ID,
				Scope:            ae.Scope,
				TriggerTimestamp: t.Timestamp,
				TriggerSource:    t.Source.String(),
				TriggerDetector:  t.DetectorName,
				TriggerType:      trigType,
				Instant:          sc.Instant,
				EWMA:             sc.EWMA,
				PreviousEWMA:     sc.PreviousEWMA,
				Severity:         string(sc.Severity),
				PreviousSeverity: string(sc.PreviousSeverity),
				SeverityChanged:  sc.SeverityChanged,
				Trend:            string(sc.Trend),
				SignalCount:      ae.Breakdown.SignalCount,
				EffectiveSignals: ae.Breakdown.EffectiveSignalCount,
			})
		}
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
		AnomalyEvents:  outAnomalyEvents,
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
