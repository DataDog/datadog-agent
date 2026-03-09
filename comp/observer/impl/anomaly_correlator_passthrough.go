// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"sort"
	"sync"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// DetectorPassthroughCorrelator passes all anomalies straight through, grouped
// by detector name. It does no time-clustering or filtering — each detector's
// anomalies become a separate ActiveCorrelation. This is used for Level 1
// detector-specific evaluation where we want to score each detector's raw
// output independently.
type DetectorPassthroughCorrelator struct {
	// anomaliesByDetector groups anomalies by DetectorName.
	anomaliesByDetector map[string][]observer.Anomaly
	mu                  sync.RWMutex
}

// NewDetectorPassthroughCorrelator creates a new DetectorPassthroughCorrelator.
func NewDetectorPassthroughCorrelator() *DetectorPassthroughCorrelator {
	return &DetectorPassthroughCorrelator{
		anomaliesByDetector: make(map[string][]observer.Anomaly),
	}
}

// Name returns the correlator name.
func (c *DetectorPassthroughCorrelator) Name() string {
	return "detector_passthrough_correlator"
}

// Process stores the anomaly, grouped by its DetectorName.
func (c *DetectorPassthroughCorrelator) Process(anomaly observer.Anomaly) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.anomaliesByDetector[anomaly.DetectorName] = append(c.anomaliesByDetector[anomaly.DetectorName], anomaly)
}

// Flush is a no-op; state is read via ActiveCorrelations.
func (c *DetectorPassthroughCorrelator) Flush() []observer.ReportOutput {
	return nil
}

// Reset clears all internal state for reanalysis.
func (c *DetectorPassthroughCorrelator) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.anomaliesByDetector = make(map[string][]observer.Anomaly)
}

// ActiveCorrelations returns one correlation per individual anomaly, sorted by
// detector name then timestamp. Each anomaly becomes its own period so the
// scorer can evaluate each detection timestamp independently.
func (c *DetectorPassthroughCorrelator) ActiveCorrelations() []observer.ActiveCorrelation {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Sort detector names for deterministic ordering
	detectorNames := make([]string, 0, len(c.anomaliesByDetector))
	for name := range c.anomaliesByDetector {
		detectorNames = append(detectorNames, name)
	}
	sort.Strings(detectorNames)

	var result []observer.ActiveCorrelation
	for _, detName := range detectorNames {
		anomalies := c.anomaliesByDetector[detName]

		// Sort anomalies by timestamp for deterministic output
		sorted := make([]observer.Anomaly, len(anomalies))
		copy(sorted, anomalies)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Timestamp < sorted[j].Timestamp })

		for i, a := range sorted {
			result = append(result, observer.ActiveCorrelation{
				Pattern:         fmt.Sprintf("passthrough_%s_%d", detName, i),
				Title:           fmt.Sprintf("Passthrough[%s]: %s", detName, a.Source),
				MemberSeriesIDs: []observer.SeriesID{a.SourceSeriesID},
				MetricNames:     []observer.MetricName{a.Source},
				Anomalies:       []observer.Anomaly{a},
				FirstSeen:       a.Timestamp,
				LastUpdated:     a.Timestamp,
			})
		}
	}

	return result
}
