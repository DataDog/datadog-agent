// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// ScrappyReporterConfig configures the scrappy state-transition reporter.
type ScrappyReporterConfig struct {
	// ConsecutiveAlertTicks is the number of consecutive alert ticks required
	// before emitting an onset event. Prevents warmup false positives.
	ConsecutiveAlertTicks int `json:"consecutive_alert_ticks"`

	// ConsecutiveNormalTicks is the number of consecutive normal ticks required
	// after an alert period before emitting a recovery event.
	ConsecutiveNormalTicks int `json:"consecutive_normal_ticks"`
}

// DefaultScrappyReporterConfig returns sensible defaults.
func DefaultScrappyReporterConfig() ScrappyReporterConfig {
	return ScrappyReporterConfig{
		ConsecutiveAlertTicks:  3,
		ConsecutiveNormalTicks: 3,
	}
}

// scrappyReporter tracks scrappy detector anomalies and produces state-transition
// events (onset and recovery). It watches raw anomalies from the scrappy detector
// and requires N consecutive alert ticks before signaling onset, and N consecutive
// normal ticks before signaling recovery.
//
// The reporter injects ActiveCorrelation entries into the ReportOutput so they
// flow through the existing EventReporter without changes.
type scrappyReporter struct {
	config ScrappyReporterConfig

	// State tracking.
	alerting         bool                 // currently in alert state
	consecutiveAlert int                  // consecutive alert ticks seen
	consecutiveNorm  int                  // consecutive normal ticks seen
	onsetTimestamp   int64                // when the current alert period started
	onsetAnomaly     *observerdef.Anomaly // the anomaly that triggered onset (carries salience)

	// Correlation counter for unique pattern names.
	correlationCount int
}

// newScrappyReporter creates a scrappy state-transition reporter.
func newScrappyReporter(config ScrappyReporterConfig) *scrappyReporter {
	return &scrappyReporter{config: config}
}

// Name returns the reporter name.
func (r *scrappyReporter) Name() string { return "scrappy_reporter" }

// Report scans NewAnomalies for scrappy detector anomalies, tracks state
// transitions, and injects onset/recovery correlations into the output for
// downstream reporters (EventReporter). Takes a pointer so injected
// correlations are visible to subsequent reporters.
func (r *scrappyReporter) Report(output *observerdef.ReportOutput) {
	// Check if scrappy fired this tick.
	var scrappyAnomaly *observerdef.Anomaly
	for i := range output.NewAnomalies {
		if output.NewAnomalies[i].DetectorName == "scrappy_detector" {
			scrappyAnomaly = &output.NewAnomalies[i]
			break
		}
	}

	hasScrappyAlert := scrappyAnomaly != nil

	if hasScrappyAlert {
		r.consecutiveAlert++
		r.consecutiveNorm = 0
	} else {
		r.consecutiveNorm++
		r.consecutiveAlert = 0
	}

	// --- Onset detection ---
	if !r.alerting && r.consecutiveAlert >= r.config.ConsecutiveAlertTicks {
		r.alerting = true
		r.correlationCount++
		// Onset timestamp is the first tick of the consecutive run.
		r.onsetTimestamp = output.AdvancedToSec - int64(r.consecutiveAlert-1)
		r.onsetAnomaly = scrappyAnomaly

		onset := r.buildOnsetCorrelation(scrappyAnomaly, output.AdvancedToSec)
		output.ActiveCorrelations = append(output.ActiveCorrelations, onset)
	}

	// --- Recovery detection ---
	if r.alerting && r.consecutiveNorm >= r.config.ConsecutiveNormalTicks {
		recovery := r.buildRecoveryCorrelation(output.AdvancedToSec)
		output.ActiveCorrelations = append(output.ActiveCorrelations, recovery)

		r.alerting = false
		r.onsetTimestamp = 0
		r.onsetAnomaly = nil
	}
}

// buildOnsetCorrelation creates an ActiveCorrelation for a scrappy alert onset.
func (r *scrappyReporter) buildOnsetCorrelation(anomaly *observerdef.Anomaly, dataTime int64) observerdef.ActiveCorrelation {
	pattern := fmt.Sprintf("scrappy_onset_%d", r.correlationCount)

	ac := observerdef.ActiveCorrelation{
		Pattern:     pattern,
		Title:       fmt.Sprintf("Scrappy: anomaly detected (P(alert)=%.3f)", *anomaly.Score),
		FirstSeen:   r.onsetTimestamp,
		LastUpdated: dataTime,
	}

	if anomaly != nil {
		ac.Anomalies = []observerdef.Anomaly{*anomaly}
		ac.Members = []observerdef.SeriesDescriptor{anomaly.Source}
	}

	return ac
}

// buildRecoveryCorrelation creates an ActiveCorrelation for a scrappy recovery.
func (r *scrappyReporter) buildRecoveryCorrelation(dataTime int64) observerdef.ActiveCorrelation {
	pattern := fmt.Sprintf("scrappy_recovery_%d", r.correlationCount)

	ac := observerdef.ActiveCorrelation{
		Pattern:     pattern,
		Title:       "Scrappy: anomaly resolved",
		FirstSeen:   dataTime - int64(r.config.ConsecutiveNormalTicks-1),
		LastUpdated: dataTime,
	}

	// Include the onset anomaly for context — what was the alert about.
	if r.onsetAnomaly != nil {
		ac.Anomalies = []observerdef.Anomaly{*r.onsetAnomaly}
		ac.Members = []observerdef.SeriesDescriptor{r.onsetAnomaly.Source}
	}

	return ac
}
