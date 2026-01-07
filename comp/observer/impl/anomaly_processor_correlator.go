// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"sort"
	"strings"
	"time"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// CorrelatorConfig configures the CrossSignalCorrelator.
type CorrelatorConfig struct {
	// WindowDuration is the time window for clustering anomalies.
	// Anomalies older than this are evicted from the buffer.
	// Default: 30 seconds.
	WindowDuration time.Duration

	// Now returns the current time. Override for testing.
	// Default: time.Now.
	Now func() time.Time
}

// DefaultCorrelatorConfig returns a CorrelatorConfig with default values.
func DefaultCorrelatorConfig() CorrelatorConfig {
	return CorrelatorConfig{
		WindowDuration: 30 * time.Second,
		Now:            time.Now,
	}
}

// timestampedAnomaly pairs an anomaly with its arrival timestamp.
type timestampedAnomaly struct {
	timestamp time.Time
	anomaly   observer.AnomalyOutput
}

// correlationPattern defines a known pattern of correlated signals.
type correlationPattern struct {
	name           string
	requiredSources []string
	reportTitle    string
}

// knownPatterns contains all known correlation patterns.
// Source names include aggregation suffixes (e.g., ":avg" for value elevation, ":count" for frequency elevation).
var knownPatterns = []correlationPattern{
	{
		name:            "kernel_bottleneck",
		requiredSources: []string{"network.retransmits:avg", "ebpf.lock_contention_ns:avg", "connection.errors:count"},
		reportTitle:     "Correlated: Kernel network bottleneck",
	},
}

// CrossSignalCorrelator clusters anomalies from different signals within a time window
// and detects known patterns. It implements CorrelationState to allow reporters to
// read the current correlation state.
type CrossSignalCorrelator struct {
	config             CorrelatorConfig
	buffer             []timestampedAnomaly
	activeCorrelations map[string]*observer.ActiveCorrelation
}

// NewCorrelator creates a new CrossSignalCorrelator with the given config.
// If config has zero values, defaults are applied.
func NewCorrelator(config CorrelatorConfig) *CrossSignalCorrelator {
	if config.WindowDuration == 0 {
		config.WindowDuration = 30 * time.Second
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &CrossSignalCorrelator{
		config:             config,
		buffer:             nil,
		activeCorrelations: make(map[string]*observer.ActiveCorrelation),
	}
}

// Name returns the processor name.
func (c *CrossSignalCorrelator) Name() string {
	return "cross_signal_correlator"
}

// Process adds an anomaly with the current timestamp to the buffer
// and evicts entries older than WindowDuration.
func (c *CrossSignalCorrelator) Process(anomaly observer.AnomalyOutput) {
	now := c.config.Now()

	// Add the new anomaly with current timestamp
	c.buffer = append(c.buffer, timestampedAnomaly{
		timestamp: now,
		anomaly:   anomaly,
	})

	// Evict old entries
	c.evictOldEntries(now)
}

// evictOldEntries removes entries older than WindowDuration from the buffer.
func (c *CrossSignalCorrelator) evictOldEntries(now time.Time) {
	cutoff := now.Add(-c.config.WindowDuration)
	newBuffer := c.buffer[:0]
	for _, entry := range c.buffer {
		if !entry.timestamp.Before(cutoff) {
			newBuffer = append(newBuffer, entry)
		}
	}
	c.buffer = newBuffer
}

// Flush checks for known patterns in the buffered anomalies and updates activeCorrelations state.
// It returns an empty slice since reporters now pull state via ActiveCorrelations() instead of
// receiving pushed reports.
func (c *CrossSignalCorrelator) Flush() []observer.ReportOutput {
	now := c.config.Now()

	// Evict old entries before checking patterns
	c.evictOldEntries(now)

	// Extract unique signal sources
	sourceSet := make(map[string]struct{})
	for _, entry := range c.buffer {
		sourceSet[entry.anomaly.Source] = struct{}{}
	}

	// Track which patterns are currently active
	currentlyActive := make(map[string]bool)

	// Check against known patterns and update state
	for _, pattern := range knownPatterns {
		if c.patternMatches(pattern, sourceSet) {
			currentlyActive[pattern.name] = true

			// Collect the anomalies that match this pattern's required sources
			matchingAnomalies := c.collectMatchingAnomalies(pattern)

			if existing, ok := c.activeCorrelations[pattern.name]; ok {
				// Pattern already active - update LastUpdated and Anomalies
				existing.LastUpdated = now
				existing.Signals = c.getSortedSources(sourceSet)
				existing.Anomalies = matchingAnomalies
			} else {
				// New pattern match - create ActiveCorrelation
				c.activeCorrelations[pattern.name] = &observer.ActiveCorrelation{
					Pattern:     pattern.name,
					Title:       pattern.reportTitle,
					Signals:     c.getSortedSources(sourceSet),
					Anomalies:   matchingAnomalies,
					FirstSeen:   now,
					LastUpdated: now,
				}
			}
		}
	}

	// Remove patterns that are no longer active (signals expired)
	for name := range c.activeCorrelations {
		if !currentlyActive[name] {
			delete(c.activeCorrelations, name)
		}
	}

	// Return empty slice - reporters pull state via ActiveCorrelations()
	return nil
}

// patternMatches checks if all required sources for a pattern are present.
func (c *CrossSignalCorrelator) patternMatches(pattern correlationPattern, sources map[string]struct{}) bool {
	for _, required := range pattern.requiredSources {
		if _, ok := sources[required]; !ok {
			return false
		}
	}
	return true
}

// collectMatchingAnomalies returns anomalies from the buffer that match the pattern's required sources,
// deduped by source - keeping only the most recent anomaly per source (it has the most complete data).
func (c *CrossSignalCorrelator) collectMatchingAnomalies(pattern correlationPattern) []observer.AnomalyOutput {
	// Map from source to most recent anomaly for that source
	bySource := make(map[string]observer.AnomalyOutput)

	for _, entry := range c.buffer {
		for _, src := range pattern.requiredSources {
			if entry.anomaly.Source == src {
				existing, exists := bySource[src]
				// Keep the one with the later End time (more recent/complete data)
				if !exists || entry.anomaly.TimeRange.End > existing.TimeRange.End {
					bySource[src] = entry.anomaly
				}
				break
			}
		}
	}

	// Convert map to slice
	result := make([]observer.AnomalyOutput, 0, len(bySource))
	for _, anomaly := range bySource {
		result = append(result, anomaly)
	}
	return result
}

// buildReport creates a ReportOutput for a matched pattern.
func (c *CrossSignalCorrelator) buildReport(pattern correlationPattern, sources map[string]struct{}) observer.ReportOutput {
	// Get sorted list of sources for consistent output
	sourceList := make([]string, 0, len(sources))
	for source := range sources {
		sourceList = append(sourceList, source)
	}
	sort.Strings(sourceList)

	return observer.ReportOutput{
		Title: pattern.reportTitle,
		Body:  "Correlated signals: " + strings.Join(sourceList, ", "),
		Metadata: map[string]string{
			"pattern":      pattern.name,
			"signal_count": "3",
		},
	}
}

// GetBuffer returns the current buffer (for testing).
func (c *CrossSignalCorrelator) GetBuffer() []timestampedAnomaly {
	return c.buffer
}

// getSortedSources returns a sorted slice of source names from a source set.
func (c *CrossSignalCorrelator) getSortedSources(sources map[string]struct{}) []string {
	sourceList := make([]string, 0, len(sources))
	for source := range sources {
		sourceList = append(sourceList, source)
	}
	sort.Strings(sourceList)
	return sourceList
}

// ActiveCorrelations returns a copy of the currently active correlation patterns.
// This implements the CorrelationState interface.
func (c *CrossSignalCorrelator) ActiveCorrelations() []observer.ActiveCorrelation {
	result := make([]observer.ActiveCorrelation, 0, len(c.activeCorrelations))
	for _, ac := range c.activeCorrelations {
		// Return a copy to prevent external modification
		anomaliesCopy := make([]observer.AnomalyOutput, len(ac.Anomalies))
		copy(anomaliesCopy, ac.Anomalies)
		result = append(result, observer.ActiveCorrelation{
			Pattern:     ac.Pattern,
			Title:       ac.Title,
			Signals:     append([]string(nil), ac.Signals...),
			Anomalies:   anomaliesCopy,
			FirstSeen:   ac.FirstSeen,
			LastUpdated: ac.LastUpdated,
		})
	}
	return result
}
