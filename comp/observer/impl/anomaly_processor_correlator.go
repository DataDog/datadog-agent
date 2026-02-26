// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"sort"
	"strings"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// CorrelatorConfig configures the CrossSignalCorrelator.
type CorrelatorConfig struct {
	// WindowSeconds is the time window (in seconds) for clustering anomalies.
	// Anomalies with data timestamps older than (currentDataTime - WindowSeconds) are evicted.
	// Default: 30 seconds.
	WindowSeconds int64
}

// DefaultCorrelatorConfig returns a CorrelatorConfig with default values.
func DefaultCorrelatorConfig() CorrelatorConfig {
	return CorrelatorConfig{
		WindowSeconds: 30,
	}
}

// timestampedAnomaly pairs an anomaly with its data timestamp.
type timestampedAnomaly struct {
	dataTime int64 // timestamp from the anomaly's data (Timestamp field)
	anomaly  observer.AnomalyOutput
}

// timestampedEventSignal pairs an event signal with its timestamp for windowing.
type timestampedEventSignal struct {
	dataTime int64 // timestamp from the event signal
	event    observer.EventSignal
}

// correlationPattern defines a known pattern of correlated signals.
type correlationPattern struct {
	name            string
	requiredSources []observer.MetricName
	reportTitle     string
}

// knownPatterns contains all known correlation patterns.
// Source names include aggregation suffixes (e.g., ":avg" for value elevation, ":count" for frequency elevation).
// Patterns are checked in order, so more specific patterns (more required sources) should come first.
var knownPatterns = []correlationPattern{
	{
		// Most specific: all 3 signals indicate kernel-level issue
		name:            "kernel_bottleneck",
		requiredSources: []observer.MetricName{"network.retransmits:avg", "ebpf.lock_contention_ns:avg", "connection.errors:count"},
		reportTitle:     "Correlated: Kernel network bottleneck",
	},
	{
		// Less specific: network issues without clear kernel involvement
		name:            "network_degradation",
		requiredSources: []observer.MetricName{"network.retransmits:avg", "connection.errors:count"},
		reportTitle:     "Correlated: Network degradation",
	},
	{
		// Lock contention causing downstream failures
		name:            "lock_contention_cascade",
		requiredSources: []observer.MetricName{"ebpf.lock_contention_ns:avg", "connection.errors:count"},
		reportTitle:     "Correlated: Lock contention cascade",
	},
}

// CrossSignalCorrelator clusters anomalies from different sources within a time window
// and detects known patterns. It implements CorrelationState, AnomalyProcessor, and
// EventSignalReceiver. Reporters read the current correlation state.
//
// Time is derived entirely from input data timestamps (anomaly.TimeRange.End),
// making the correlator deterministic with respect to input data.
type CrossSignalCorrelator struct {
	config             CorrelatorConfig
	buffer             []timestampedAnomaly
	eventSignals       []timestampedEventSignal // Event signals for correlation context
	activeCorrelations map[string]*observer.ActiveCorrelation
	currentDataTime    int64 // latest data timestamp seen
}

// NewCorrelator creates a new CrossSignalCorrelator with the given config.
// If config has zero values, defaults are applied.
func NewCorrelator(config CorrelatorConfig) *CrossSignalCorrelator {
	if config.WindowSeconds == 0 {
		config.WindowSeconds = 30
	}
	return &CrossSignalCorrelator{
		config:             config,
		buffer:             nil,
		activeCorrelations: make(map[string]*observer.ActiveCorrelation),
		currentDataTime:    0,
	}
}

// Name returns the processor name.
func (c *CrossSignalCorrelator) Name() string {
	return "cross_signal_correlator"
}

// Process implements AnomalyProcessor (old interface). It adds an anomaly to the buffer
// using its data timestamp (TimeRange.End) and evicts old entries.
func (c *CrossSignalCorrelator) Process(anomaly observer.AnomalyOutput) {
	dataTime := anomaly.Timestamp

	// Update current data time (monotonically advancing)
	if dataTime > c.currentDataTime {
		c.currentDataTime = dataTime
	}

	// Add the new anomaly with its data timestamp
	c.buffer = append(c.buffer, timestampedAnomaly{
		dataTime: dataTime,
		anomaly:  anomaly,
	})

	// Evict old entries based on data time
	c.evictOldEntries()
}

// AddEventSignal implements EventSignalReceiver. It adds an event signal for correlation context.
func (c *CrossSignalCorrelator) AddEventSignal(event observer.EventSignal) {
	dataTime := event.Timestamp

	// Update current data time (monotonically advancing)
	if dataTime > c.currentDataTime {
		c.currentDataTime = dataTime
	}

	// Add the event signal with its timestamp
	c.eventSignals = append(c.eventSignals, timestampedEventSignal{
		dataTime: dataTime,
		event:    event,
	})

	// Evict old entries based on data time
	c.evictOldEntries()
}

// evictOldEntries removes entries older than WindowSeconds from all buffers.
func (c *CrossSignalCorrelator) evictOldEntries() {
	cutoff := c.currentDataTime - c.config.WindowSeconds

	// Evict old anomalies
	newBuffer := c.buffer[:0]
	for _, entry := range c.buffer {
		if entry.dataTime >= cutoff {
			newBuffer = append(newBuffer, entry)
		}
	}
	c.buffer = newBuffer

	// Evict old event signals
	newEventSignals := c.eventSignals[:0]
	for _, entry := range c.eventSignals {
		if entry.dataTime >= cutoff {
			newEventSignals = append(newEventSignals, entry)
		}
	}
	c.eventSignals = newEventSignals
}

// Flush implements AnomalyProcessor. It checks for known patterns in the anomaly buffer
// and updates activeCorrelations state. Returns empty slice since reporters pull state via ActiveCorrelations().
func (c *CrossSignalCorrelator) Flush() []observer.ReportOutput {
	// Evict old entries before checking patterns
	c.evictOldEntries()

	// Extract unique anomaly sources
	sourceSet := make(map[observer.MetricName]struct{})
	for _, entry := range c.buffer {
		sourceSet[entry.anomaly.Source] = struct{}{}
	}

	// Track which patterns are currently active
	currentlyActive := make(map[string]bool)

	// Check against known patterns and update state
	for _, pattern := range knownPatterns {
		if c.patternMatches(pattern, sourceSet) {
			currentlyActive[pattern.name] = true

			// Collect matching anomalies from old buffer for backward compatibility
			matchingAnomalies := c.collectMatchingAnomalies(pattern)

			// Collect event signals in the current window
			eventSignals := c.collectEventSignalsInWindow()

			if existing, ok := c.activeCorrelations[pattern.name]; ok {
				// Pattern already active - update LastUpdated and Anomalies
				existing.LastUpdated = c.currentDataTime
				existing.Anomalies = matchingAnomalies
				existing.MemberSeriesIDs = sortedUniqueSeriesIDs(matchingAnomalies)
				existing.MetricNames = c.getSortedMetricNames(sourceSet)
				existing.EventSignals = eventSignals
			} else {
				// New pattern match - create ActiveCorrelation
				c.activeCorrelations[pattern.name] = &observer.ActiveCorrelation{
					Pattern:         pattern.name,
					Title:           pattern.reportTitle,
					MemberSeriesIDs: sortedUniqueSeriesIDs(matchingAnomalies),
					MetricNames:     c.getSortedMetricNames(sourceSet),
					Anomalies:       matchingAnomalies, // Populated from old buffer for backward compat
					EventSignals:    eventSignals,
					FirstSeen:       c.currentDataTime,
					LastUpdated:     c.currentDataTime,
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
func (c *CrossSignalCorrelator) patternMatches(pattern correlationPattern, sources map[observer.MetricName]struct{}) bool {
	for _, required := range pattern.requiredSources {
		if _, ok := sources[required]; !ok {
			return false
		}
	}
	return true
}

// collectMatchingAnomalies returns anomalies from the buffer that match the pattern's required sources,
// deduped by source - keeping only the most recent anomaly per source.
func (c *CrossSignalCorrelator) collectMatchingAnomalies(pattern correlationPattern) []observer.AnomalyOutput {
	// Map from source to most recent anomaly for that source
	bySource := make(map[observer.MetricName]observer.AnomalyOutput)

	for _, entry := range c.buffer {
		for _, src := range pattern.requiredSources {
			if entry.anomaly.Source == src {
				existing, exists := bySource[src]
				// Keep the one with the later timestamp (more recent)
				if !exists || entry.anomaly.Timestamp > existing.Timestamp {
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

// collectEventSignalsInWindow returns all event signals currently in the window.
// Event signals are sorted by timestamp (oldest first) for consistent output.
func (c *CrossSignalCorrelator) collectEventSignalsInWindow() []observer.EventSignal {
	if len(c.eventSignals) == 0 {
		return nil
	}
	result := make([]observer.EventSignal, len(c.eventSignals))
	for i, es := range c.eventSignals {
		result[i] = es.event
	}
	// Sort by timestamp for deterministic output
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp < result[j].Timestamp
	})
	return result
}

// buildReport creates a ReportOutput for a matched pattern.
func (c *CrossSignalCorrelator) buildReport(pattern correlationPattern, sources map[observer.MetricName]struct{}) observer.ReportOutput {
	// Get sorted list of sources for consistent output
	sourceList := make([]string, 0, len(sources))
	for source := range sources {
		sourceList = append(sourceList, string(source))
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

// getSortedMetricNames returns a sorted slice of metric names from a source set.
func (c *CrossSignalCorrelator) getSortedMetricNames(sources map[observer.MetricName]struct{}) []observer.MetricName {
	sourceList := make([]observer.MetricName, 0, len(sources))
	for source := range sources {
		sourceList = append(sourceList, source)
	}
	sort.Slice(sourceList, func(i, j int) bool { return sourceList[i] < sourceList[j] })
	return sourceList
}

// ActiveCorrelations returns a copy of the currently active correlation patterns.
// This implements the CorrelationState interface.
func (c *CrossSignalCorrelator) ActiveCorrelations() []observer.ActiveCorrelation {
	result := make([]observer.ActiveCorrelation, 0, len(c.activeCorrelations))
	for _, ac := range c.activeCorrelations {
		// Return a copy to prevent external modification
		result = append(result, observer.ActiveCorrelation{
			Pattern:         ac.Pattern,
			Title:           ac.Title,
			MemberSeriesIDs: append([]observer.SeriesID(nil), ac.MemberSeriesIDs...),
			MetricNames:     append([]observer.MetricName(nil), ac.MetricNames...),
			Anomalies:       ac.Anomalies, // Populated from old AnomalyOutput buffer for backward compat
			EventSignals:    append([]observer.EventSignal(nil), ac.EventSignals...),
			FirstSeen:       ac.FirstSeen,
			LastUpdated:     ac.LastUpdated,
		})
	}
	return result
}
