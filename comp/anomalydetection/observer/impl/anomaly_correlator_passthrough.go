// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"sort"
	"sync"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// DetectorPassthroughCorrelator passes all anomalies straight through without
// time-clustering or filtering. Each individual anomaly becomes its own
// ActiveCorrelation. This is used for Level 1 detector-specific evaluation
// where we want to score each detector's raw output independently.
type DetectorPassthroughCorrelator struct {
	// anomaliesByDetector groups anomalies by DetectorName.
	anomaliesByDetector map[string][]observer.Anomaly
	mu                  sync.RWMutex
	emitter             correlationEmitter
}

// NewDetectorPassthroughCorrelator creates a new DetectorPassthroughCorrelator.
func NewDetectorPassthroughCorrelator() *DetectorPassthroughCorrelator {
	return &DetectorPassthroughCorrelator{
		anomaliesByDetector: make(map[string][]observer.Anomaly),
		emitter:             newCorrelationEmitter("passthrough_correlator"),
	}
}

// Name returns the correlator name.
func (c *DetectorPassthroughCorrelator) Name() string {
	return "passthrough_correlator"
}

// ProcessAnomaly stores the anomaly, grouped by its DetectorName.
func (c *DetectorPassthroughCorrelator) ProcessAnomaly(anomaly observer.Anomaly) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.anomaliesByDetector[anomaly.DetectorName] = append(c.anomaliesByDetector[anomaly.DetectorName], anomaly)
}

// Advance observes the current active correlations for emission.
func (c *DetectorPassthroughCorrelator) Advance(dataTime int64) {
	c.mu.RLock()
	active := c.activeCorrelationsLocked()
	c.mu.RUnlock()
	c.emitter.observe(active, dataTime)
}

// Reset clears all internal state for reanalysis.
func (c *DetectorPassthroughCorrelator) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.anomaliesByDetector = make(map[string][]observer.Anomaly)
	c.emitter.reset()
}

// PendingEvents drains CorrelationDetected events accumulated during the last Advance.
func (c *DetectorPassthroughCorrelator) PendingEvents() []observer.CorrelatorEvent {
	return c.emitter.drain()
}

// activeCorrelationsLocked builds active correlations from current state.
// Caller must hold c.mu (at least read lock).
func (c *DetectorPassthroughCorrelator) activeCorrelationsLocked() []observer.ActiveCorrelation {
	detectorNames := make([]string, 0, len(c.anomaliesByDetector))
	for name := range c.anomaliesByDetector {
		detectorNames = append(detectorNames, name)
	}
	sort.Strings(detectorNames)

	var result []observer.ActiveCorrelation
	for _, detName := range detectorNames {
		anomalies := c.anomaliesByDetector[detName]

		sorted := make([]observer.Anomaly, len(anomalies))
		copy(sorted, anomalies)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Timestamp < sorted[j].Timestamp })

		for i, a := range sorted {
			result = append(result, observer.ActiveCorrelation{
				Pattern:     fmt.Sprintf("passthrough_%s_%d", detName, i),
				Title:       fmt.Sprintf("Passthrough[%s]: %s", detName, a.Source),
				Members:     []observer.SeriesDescriptor{a.Source},
				Anomalies:   []observer.Anomaly{a},
				FirstSeen:   a.Timestamp,
				LastUpdated: a.Timestamp,
			})
		}
	}
	return result
}

// ActiveCorrelations returns one ActiveCorrelation per individual anomaly,
// sorted by detector name then timestamp. Each anomaly becomes its own
// correlation, allowing the scorer to evaluate each detection independently.
func (c *DetectorPassthroughCorrelator) ActiveCorrelations() []observer.ActiveCorrelation {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.activeCorrelationsLocked()
}
