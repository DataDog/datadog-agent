// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"sync"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// ScrappyCorrelatorConfig configures the scrappy state-transition correlator.
type ScrappyCorrelatorConfig struct {
	// ConsecutiveAlertTicks is the number of consecutive alert ticks required
	// before emitting an onset correlation. Prevents warmup false positives.
	ConsecutiveAlertTicks int `json:"consecutive_alert_ticks"`

	// ConsecutiveNormalTicks is the number of consecutive normal ticks required
	// after an alert period before emitting a recovery correlation.
	ConsecutiveNormalTicks int `json:"consecutive_normal_ticks"`
}

// DefaultScrappyCorrelatorConfig returns sensible defaults (3 ticks each).
func DefaultScrappyCorrelatorConfig() ScrappyCorrelatorConfig {
	return ScrappyCorrelatorConfig{
		ConsecutiveAlertTicks:  3,
		ConsecutiveNormalTicks: 3,
	}
}

// ScrappyCorrelator watches scrappy_detector anomalies and produces onset /
// recovery ActiveCorrelation entries. It requires N consecutive alert ticks
// before declaring an onset, and M consecutive normal ticks before declaring
// a recovery.
//
// Implements observerdef.Correlator so it integrates with the existing engine
// pipeline: the engine fans ProcessAnomaly to all correlators after each detect
// cycle, then collects ActiveCorrelations to pass to reporters.
type ScrappyCorrelator struct {
	config ScrappyCorrelatorConfig
	mu     sync.Mutex

	// Alert state.
	alerting         bool
	consecutiveAlert int
	consecutiveNorm  int
	onsetTimestamp   int64
	onsetAnomaly     *observerdef.Anomaly
	correlationCount int
	lastDataTime     int64

	// accumulated correlations since last Reset.
	correlations []observerdef.ActiveCorrelation
}

// NewScrappyCorrelator creates a new correlator with the given config.
func NewScrappyCorrelator(config ScrappyCorrelatorConfig) *ScrappyCorrelator {
	return &ScrappyCorrelator{config: config}
}

// Name returns the correlator name.
func (c *ScrappyCorrelator) Name() string { return "scrappy_correlator" }

// ProcessAnomaly receives an anomaly event. Only anomalies from the
// scrappy_detector are acted on; all others are silently ignored.
func (c *ScrappyCorrelator) ProcessAnomaly(a observerdef.Anomaly) {
	if a.DetectorName != "scrappy_detector" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.consecutiveAlert++
	c.consecutiveNorm = 0
	an := a
	c.onsetAnomaly = &an
	c.lastDataTime = a.Timestamp
}

// Advance performs time-based maintenance. It records each data-time tick to
// track consecutive normal ticks even when scrappy_detector fires no anomaly.
func (c *ScrappyCorrelator) Advance(dataTime int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if dataTime == c.lastDataTime {
		// Already processed this tick via ProcessAnomaly — skip.
		// (Advance is called after ProcessAnomaly for the same dataTime.)
	} else {
		// No scrappy anomaly this tick.
		c.consecutiveNorm++
		c.consecutiveAlert = 0
		c.lastDataTime = dataTime
	}

	// Onset: N consecutive alert ticks crossed.
	if !c.alerting && c.consecutiveAlert >= c.config.ConsecutiveAlertTicks {
		c.alerting = true
		c.correlationCount++
		c.onsetTimestamp = dataTime - int64(c.consecutiveAlert-1)
		onset := c.buildOnsetCorrelation(c.onsetAnomaly, dataTime)
		c.correlations = append(c.correlations, onset)
	}

	// Recovery: M consecutive normal ticks after being in alert state.
	if c.alerting && c.consecutiveNorm >= c.config.ConsecutiveNormalTicks {
		recovery := c.buildRecoveryCorrelation(dataTime)
		c.correlations = append(c.correlations, recovery)
		c.alerting = false
		c.onsetTimestamp = 0
		c.onsetAnomaly = nil
	}
}

// ActiveCorrelations returns current onset/recovery correlations.
func (c *ScrappyCorrelator) ActiveCorrelations() []observerdef.ActiveCorrelation {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]observerdef.ActiveCorrelation, len(c.correlations))
	copy(result, c.correlations)
	return result
}

// Reset clears all internal state for reanalysis.
func (c *ScrappyCorrelator) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.alerting = false
	c.consecutiveAlert = 0
	c.consecutiveNorm = 0
	c.onsetTimestamp = 0
	c.onsetAnomaly = nil
	c.correlationCount = 0
	c.lastDataTime = 0
	c.correlations = nil
}

func (c *ScrappyCorrelator) buildOnsetCorrelation(anomaly *observerdef.Anomaly, dataTime int64) observerdef.ActiveCorrelation {
	pattern := fmt.Sprintf("scrappy_onset_%d", c.correlationCount)

	var scoreStr string
	if anomaly != nil && anomaly.Score != nil {
		scoreStr = fmt.Sprintf("%.3f", *anomaly.Score)
	}

	ac := observerdef.ActiveCorrelation{
		Pattern:     pattern,
		Title:       fmt.Sprintf("Scrappy: anomaly detected (P(alert)=%s)", scoreStr),
		FirstSeen:   c.onsetTimestamp,
		LastUpdated: dataTime,
	}

	if anomaly != nil {
		ac.Anomalies = []observerdef.Anomaly{*anomaly}
		ac.Members = []observerdef.SeriesDescriptor{anomaly.Source}
	}

	return ac
}

func (c *ScrappyCorrelator) buildRecoveryCorrelation(dataTime int64) observerdef.ActiveCorrelation {
	pattern := fmt.Sprintf("scrappy_recovery_%d", c.correlationCount)

	ac := observerdef.ActiveCorrelation{
		Pattern:     pattern,
		Title:       "Scrappy: anomaly resolved",
		FirstSeen:   dataTime - int64(c.config.ConsecutiveNormalTicks-1),
		LastUpdated: dataTime,
	}

	if c.onsetAnomaly != nil {
		ac.Anomalies = []observerdef.Anomaly{*c.onsetAnomaly}
		ac.Members = []observerdef.SeriesDescriptor{c.onsetAnomaly.Source}
	}

	return ac
}
