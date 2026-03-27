// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"strconv"
	"sync"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// DensityConfig configures the DensityCorrelator.
type DensityConfig struct {
	// ShortWindowSec is the burst detection window in seconds.
	// Default: 10
	ShortWindowSec int64

	// LongWindowSec is the rolling baseline window in seconds.
	// Default: 300
	LongWindowSec int64

	// MinBurst is the absolute minimum anomaly count in the short window
	// to trigger a burst event.
	// Default: 5
	MinBurst int

	// BurstMultiplier is the factor above the long-window rate required to trigger.
	// Default: 2.0
	BurstMultiplier float64

	// MinUniqueSources is the minimum number of distinct metric sources in the
	// short window to trigger a burst. Separates correlated incidents (many
	// different metrics changing) from single-metric noise (same metric across pods).
	// Default: 5
	MinUniqueSources int

	// WindowSeconds is how long to keep detected bursts before eviction.
	// Follows the same pattern as TimeCluster's WindowSeconds.
	// Default: 120
	WindowSeconds int64
}

// DefaultDensityConfig returns a DensityConfig with default values.
func DefaultDensityConfig() DensityConfig {
	return DensityConfig{
		ShortWindowSec:   10,
		LongWindowSec:    300,
		MinBurst:         5,
		BurstMultiplier:  2.0,
		MinUniqueSources: 5,
		WindowSeconds:    120,
	}
}

// readDensityConfig reads Density settings from the agent config.
func readDensityConfig(reader ConfigReader, prefix string) any {
	cfg := DefaultDensityConfig()
	if key := prefix + "min_unique_sources"; reader.IsKnown(key) {
		cfg.MinUniqueSources = reader.GetInt(key)
	}
	if key := prefix + "min_burst"; reader.IsKnown(key) {
		cfg.MinBurst = reader.GetInt(key)
	}
	if key := prefix + "short_window_sec"; reader.IsKnown(key) {
		cfg.ShortWindowSec = int64(reader.GetInt(key))
	}
	if key := prefix + "long_window_sec"; reader.IsKnown(key) {
		cfg.LongWindowSec = int64(reader.GetInt(key))
	}
	return cfg
}

// densityBurst is an active burst in the sliding window.
type densityBurst struct {
	id                  int
	windowStart         int64 // triggerTS - ShortWindowSec
	windowEnd           int64 // triggerTS
	anomalies           []observer.Anomaly
	triggerTS           int64 // the timestamp that triggered detection
	maxSamplingInterval int64 // max SamplingIntervalSec across burst members
}

// DensityCorrelator detects anomaly bursts by comparing the short-window
// anomaly rate against a rolling long-window baseline.
//
// Follows the same lifecycle pattern as TimeClusterCorrelator:
// - ProcessAnomaly: receives raw anomalies, checks for bursts
// - ActiveCorrelations: returns bursts in the sliding window
// - Advance: evicts old bursts and anomalies
// - Engine's accumulateCorrelations handles durable history
//
// Designed for scan detectors (ScanMW, ScanWelch) that produce anomalies
// with historical changepoint timestamps.
type DensityCorrelator struct {
	config DensityConfig

	mu              sync.RWMutex
	anomalies       []observer.Anomaly // raw anomalies in the long window
	bursts          []*densityBurst    // active bursts in the sliding window
	currentDataTime int64              // latest anomaly timestamp
	nextBurstID     int
}

// NewDensityCorrelator creates a DensityCorrelator with the given config.
func NewDensityCorrelator(config DensityConfig) *DensityCorrelator {
	if config.ShortWindowSec <= 0 {
		config.ShortWindowSec = 10
	}
	if config.LongWindowSec <= 0 {
		config.LongWindowSec = 300
	}
	if config.MinBurst <= 0 {
		config.MinBurst = 5
	}
	if config.BurstMultiplier <= 0 {
		config.BurstMultiplier = 2.0
	}
	if config.MinUniqueSources <= 0 {
		config.MinUniqueSources = 5
	}
	if config.WindowSeconds <= 0 {
		config.WindowSeconds = 120
	}
	return &DensityCorrelator{
		config: config,
	}
}

// Name returns the correlator name.
func (c *DensityCorrelator) Name() string {
	return "density"
}

// ProcessAnomaly appends an anomaly and checks for a burst at the anomaly's
// own timestamp. Anomalies are delivered in sorted timestamp order by the
// engine's step-advance, so a single check per anomaly is sufficient.
func (c *DensityCorrelator) ProcessAnomaly(anomaly observer.Anomaly) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.anomalies = append(c.anomalies, anomaly)
	if anomaly.Timestamp > c.currentDataTime {
		c.currentDataTime = anomaly.Timestamp
	}

	c.checkBurstLocked(anomaly.Timestamp, anomaly.SamplingIntervalSec)
}

// Advance evicts old anomalies and old bursts from the sliding window.
// Does NOT update currentDataTime from dataTime — batch detectors produce
// anomalies with historical timestamps that lag behind the data stream.
// Eviction is based on the anomaly-driven currentDataTime instead.
func (c *DensityCorrelator) Advance(_ int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict old anomalies from the long window.
	if c.currentDataTime > 0 {
		cutoff := c.currentDataTime - c.config.LongWindowSec
		kept := c.anomalies[:0]
		for _, a := range c.anomalies {
			if a.Timestamp >= cutoff {
				kept = append(kept, a)
			}
		}
		c.anomalies = kept
	}

	// Evict old bursts from the sliding window.
	burstCutoff := c.currentDataTime - c.config.WindowSeconds
	keptBursts := c.bursts[:0]
	for _, b := range c.bursts {
		if b.triggerTS >= burstCutoff {
			keptBursts = append(keptBursts, b)
		}
	}
	c.bursts = keptBursts
}

// checkBurstLocked checks whether anomalies near triggerTS form a burst.
// All anomalies in [triggerTS-ShortWindowSec, triggerTS] are included in the
// burst as evidence; FirstSeen is set to triggerTS (the moment the threshold
// was crossed), not the earliest anomaly in the window.
//
// incomingSamplingInterval is the SamplingIntervalSec of the anomaly that
// triggered this check. It widens the overlap extension window so that slow-
// sampling series (e.g. 15s redis check) can merge into bursts formed by
// faster-sampling series (e.g. 10s trace stats). Pass 0 when called for the
// max-timestamp re-check (no specific anomaly driving the call).
func (c *DensityCorrelator) checkBurstLocked(triggerTS int64, incomingSamplingInterval int64) {
	shortCutoff := triggerTS - c.config.ShortWindowSec

	// Check for overlap with existing bursts. If the trigger falls within an
	// existing burst's window, add the latest anomaly to that burst (so late-arriving
	// anomalies like redis metrics are captured) rather than creating a new burst.
	// Extension = max(ShortWindowSec, burst.maxSamplingInterval, incomingSamplingInterval)
	// so that slow-sampling anomalies can reach bursts formed by faster-sampling ones.
	for _, b := range c.bursts {
		extension := c.config.ShortWindowSec
		if b.maxSamplingInterval > extension {
			extension = b.maxSamplingInterval
		}
		if incomingSamplingInterval > extension {
			extension = incomingSamplingInterval
		}
		if triggerTS >= b.windowStart && triggerTS <= b.windowEnd+extension {
			// Find anomalies at triggerTS that aren't already in the burst.
			existing := make(map[string]bool)
			for _, a := range b.anomalies {
				existing[string(a.SourceSeriesID)+strconv.FormatInt(a.Timestamp, 10)] = true
			}
			for _, a := range c.anomalies {
				if a.Timestamp >= b.windowStart && a.Timestamp <= b.windowEnd {
					key := string(a.SourceSeriesID) + strconv.FormatInt(a.Timestamp, 10)
					if !existing[key] {
						b.anomalies = append(b.anomalies, a)
						existing[key] = true
						if a.SamplingIntervalSec > b.maxSamplingInterval {
							b.maxSamplingInterval = a.SamplingIntervalSec
						}
					}
				}
			}
			return
		}
	}

	// Count all anomalies in the short window.
	var shortCount int
	for _, a := range c.anomalies {
		if a.Timestamp >= shortCutoff && a.Timestamp <= triggerTS {
			shortCount++
		}
	}

	// Baseline rate from the long window excluding the short window.
	var longOnlyCount int
	longCutoff := triggerTS - c.config.LongWindowSec
	for _, a := range c.anomalies {
		if a.Timestamp >= longCutoff && a.Timestamp < shortCutoff {
			longOnlyCount++
		}
	}
	longOnlyDuration := c.config.LongWindowSec - c.config.ShortWindowSec
	var longRate float64
	if longOnlyDuration > 0 {
		longRate = float64(longOnlyCount) / float64(longOnlyDuration)
	}

	expectedInShort := longRate * float64(c.config.ShortWindowSec) * c.config.BurstMultiplier
	threshold := expectedInShort
	if threshold < float64(c.config.MinBurst) {
		threshold = float64(c.config.MinBurst)
	}

	if float64(shortCount) < threshold {
		return
	}

	// Diversity check.
	if c.config.MinUniqueSources > 0 {
		uniqueSources := make(map[string]bool)
		for _, a := range c.anomalies {
			if a.Timestamp >= shortCutoff && a.Timestamp <= triggerTS {
				uniqueSources[a.Source.String()] = true
			}
		}
		if len(uniqueSources) < c.config.MinUniqueSources {
			return
		}
	}

	// Burst qualifies. Include all anomalies in the short window for context.
	// FirstSeen (triggerTS) determines scoring; anomaly list is for consumers.
	var burstAnomalies []observer.Anomaly
	var maxInterval int64
	for _, a := range c.anomalies {
		if a.Timestamp >= shortCutoff && a.Timestamp <= triggerTS {
			burstAnomalies = append(burstAnomalies, a)
			if a.SamplingIntervalSec > maxInterval {
				maxInterval = a.SamplingIntervalSec
			}
		}
	}

	c.nextBurstID++
	c.bursts = append(c.bursts, &densityBurst{
		id:                  c.nextBurstID,
		windowStart:         shortCutoff,
		windowEnd:           triggerTS,
		anomalies:           burstAnomalies,
		triggerTS:           triggerTS,
		maxSamplingInterval: maxInterval,
	})
}

// ActiveCorrelations returns bursts currently in the sliding window.
// The engine's accumulateCorrelations captures these for durable history.
func (c *DensityCorrelator) ActiveCorrelations() []observer.ActiveCorrelation {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.bursts) == 0 {
		return nil
	}

	result := make([]observer.ActiveCorrelation, len(c.bursts))
	for i, b := range c.bursts {
		latestTS := b.triggerTS
		for _, a := range b.anomalies {
			if a.Timestamp > latestTS {
				latestTS = a.Timestamp
			}
		}

		result[i] = observer.ActiveCorrelation{
			Pattern:         fmt.Sprintf("density_burst_%d", b.id),
			Title:           fmt.Sprintf("Anomaly rate spike detected (trigger ts=%d, %d anomalies in window)", b.triggerTS, len(b.anomalies)),
			MemberSeriesIDs: sortedUniqueSeriesIDs(b.anomalies),
			MetricNames:     sortedUniqueMetricNames(b.anomalies),
			Anomalies:       b.anomalies,
			FirstSeen:       b.triggerTS,
			LastUpdated:     latestTS,
		}
	}
	return result
}

// Reset clears all internal state for reanalysis.
func (c *DensityCorrelator) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.anomalies = nil
	c.bursts = nil
	c.currentDataTime = 0
	c.nextBurstID = 0
}
