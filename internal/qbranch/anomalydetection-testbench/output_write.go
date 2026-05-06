// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	observerimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl"
	tboutput "github.com/DataDog/datadog-agent/internal/qbranch/anomalydetection-testbench/output"
)

// WriteObserverOutput collects correlations and metadata from the TestBench
// and writes a structured JSON results file.
// When verbose is true, correlations include title, member series, and nested anomalies.
// When verbose is false, correlations include only the time span (pattern, period_start, period_end).
func (tb *TestBench) WriteObserverOutput(path string, verbose bool) error {
	tb.mu.RLock()
	correlations := tb.engine.StateView().CorrelationHistory()

	scenario := tb.loadedScenario
	timelineStart, timelineEnd, hasBounds := tb.engine.Storage().TimeBounds()

	// Collect enabled detector / correlator names and build the full component config map.
	var detectorNames []string
	var correlatorNames []string
	componentConfigs := make(map[string]map[string]any, len(tb.components))
	for name, ci := range tb.components {
		entry := map[string]any{"enabled": ci.Enabled()}
		if ci.ActiveConfig() != nil {
			if raw, err := json.Marshal(ci.ActiveConfig()); err == nil {
				var fields map[string]any
				if err := json.Unmarshal(raw, &fields); err == nil {
					for k, v := range fields {
						entry[k] = v
					}
				}
			}
		}
		componentConfigs[name] = entry

		if !ci.Enabled() {
			continue
		}
		switch ci.Kind() {
		case observerimpl.ComponentDetector:
			detectorNames = append(detectorNames, name)
		case observerimpl.ComponentCorrelator:
			correlatorNames = append(correlatorNames, name)
		}
	}
	replayStats := tb.replayStats
	tb.mu.RUnlock()

	sort.Strings(detectorNames)
	sort.Strings(correlatorNames)

	if !hasBounds {
		timelineStart = 0
		timelineEnd = 0
	}

	// Build output correlations
	outCorrelations := make([]tboutput.ObserverCorrelation, len(correlations))
	for i, corr := range correlations {
		oc := tboutput.ObserverCorrelation{
			Pattern:     corr.Pattern,
			PeriodStart: corr.FirstSeen,
			PeriodEnd:   corr.LastUpdated,
		}

		if verbose {
			oc.Title = corr.Title
			oc.Message = observerimpl.BuildChangeMessage(corr, tb.engine.Storage())
			oc.Tags = []string{"source:agent-q-branch-observer", "pattern:" + corr.Pattern}
			oc.MemberSeries = make([]string, len(corr.Members))
			for j, m := range corr.Members {
				oc.MemberSeries[j] = m.DisplayName()
			}
			oc.Anomalies = make([]tboutput.ObserverAnomaly, len(corr.Anomalies))
			for j, a := range corr.Anomalies {
				sourceID := a.Source.Key()
				if a.SourceRef != nil {
					sourceID = a.SourceRef.CompactID()
				}
				oc.Anomalies[j] = tboutput.ObserverAnomaly{
					Timestamp:      a.Timestamp,
					Source:         a.Source.String(),
					SourceSeriesID: sourceID,
					Detector:       a.DetectorName,
				}
			}
		}

		outCorrelations[i] = oc
	}

	output := tboutput.ObserverOutput{
		Metadata: tboutput.ObserverMetadata{
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

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling observer output: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing observer output to %s: %w", path, err)
	}

	return nil
}
