// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"sort"

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
	signal   observer.EventSignal
}

// correlationPattern defines a known pattern of correlated signals.
type correlationPattern struct {
	name            string
	requiredSources []string
	reportTitle     string
}

// knownPatterns contains all known correlation patterns.
// Source names include aggregation suffixes (e.g., ":avg" for value elevation, ":count" for frequency elevation).
// Patterns are checked in order, so more specific patterns (more required sources) should come first.
var knownPatterns = []correlationPattern{
	{
		// Most specific: all 3 signals indicate kernel-level issue
		name:            "kernel_bottleneck",
		requiredSources: []string{"network.retransmits:avg", "ebpf.lock_contention_ns:avg", "connection.errors:count"},
		reportTitle:     "Correlated: Kernel network bottleneck",
	},
	{
		// Less specific: network issues without clear kernel involvement
		name:            "network_degradation",
		requiredSources: []string{"network.retransmits:avg", "connection.errors:count"},
		reportTitle:     "Correlated: Network degradation",
	},
	{
		// Lock contention causing downstream failures
		name:            "lock_contention_cascade",
		requiredSources: []string{"ebpf.lock_contention_ns:avg", "connection.errors:count"},
		reportTitle:     "Correlated: Lock contention cascade",
	},
}

// CrossSignalCorrelator clusters anomalies from different signals within a time window
// and detects known patterns. It implements CorrelationState to allow reporters to
// read the current correlation state.
//
// Time is derived entirely from input data timestamps (anomaly.Timestamp), making
// the correlator deterministic with respect to input data.
type CrossSignalCorrelator struct {
	config             CorrelatorConfig
	buffer             []timestampedAnomaly
	eventSignals       []timestampedEventSignal // discrete event signals (OOM, restarts, etc.)
	activeCorrelations map[string]*observer.ActiveCorrelation
	currentDataTime    int64 // latest data timestamp seen (max of all Timestamp values)
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

// Process adds an anomaly to the buffer using its data timestamp
// and evicts entries older than WindowSeconds.
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

// evictOldEntries removes entries older than WindowSeconds from the buffer.
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
	newSignals := c.eventSignals[:0]
	for _, entry := range c.eventSignals {
		if entry.dataTime >= cutoff {
			newSignals = append(newSignals, entry)
		}
	}
	c.eventSignals = newSignals
}

// AddEventSignal adds a discrete event signal (e.g., container OOM, restart) to the correlator.
// Event signals are used as correlation evidence/annotations but are not analyzed with CUSUM.
// They are included in active correlations to provide context about discrete events
// that occurred within the correlation window.
func (c *CrossSignalCorrelator) AddEventSignal(signal observer.EventSignal) {
	// Update current data time if this signal is more recent
	if signal.Timestamp > c.currentDataTime {
		c.currentDataTime = signal.Timestamp
	}

	// Add the event signal with its timestamp
	c.eventSignals = append(c.eventSignals, timestampedEventSignal{
		dataTime: signal.Timestamp,
		signal:   signal,
	})

	// Evict old entries (both anomalies and event signals)
	c.evictOldEntries()
}

// Flush checks for known patterns in the buffered anomalies and updates activeCorrelations state.
// It returns an empty slice since reporters now pull state via ActiveCorrelations() instead of
// receiving pushed reports.
func (c *CrossSignalCorrelator) Flush() []observer.ReportOutput {
	// Evict old entries before checking patterns
	c.evictOldEntries()

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

			// Collect all event signals within the window
			currentEventSignals := c.collectEventSignalsInWindow()

			if existing, ok := c.activeCorrelations[pattern.name]; ok {
				// Pattern already active - update LastUpdated and Anomalies
				existing.LastUpdated = c.currentDataTime
				existing.Signals = c.getSortedSources(sourceSet)
				existing.Anomalies = matchingAnomalies
				existing.EventSignals = currentEventSignals
			} else {
				// New pattern match - create ActiveCorrelation
				c.activeCorrelations[pattern.name] = &observer.ActiveCorrelation{
					Pattern:      pattern.name,
					Title:        pattern.reportTitle,
					Signals:      c.getSortedSources(sourceSet),
					Anomalies:    matchingAnomalies,
					EventSignals: currentEventSignals,
					FirstSeen:    c.currentDataTime,
					LastUpdated:  c.currentDataTime,
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

// collectEventSignalsInWindow returns all event signals currently in the window.
// Event signals are sorted by timestamp (oldest first) for consistent output.
func (c *CrossSignalCorrelator) collectEventSignalsInWindow() []observer.EventSignal {
	if len(c.eventSignals) == 0 {
		return nil
	}
	result := make([]observer.EventSignal, len(c.eventSignals))
	for i, es := range c.eventSignals {
		result[i] = es.signal
	}
	// Sort by timestamp for deterministic output
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp < result[j].Timestamp
	})
	return result
}

// collectMatchingAnomalies returns anomalies from the buffer that match the pattern's required sources,
// deduped by source - keeping only the most recent anomaly per source.
func (c *CrossSignalCorrelator) collectMatchingAnomalies(pattern correlationPattern) []observer.AnomalyOutput {
	// Map from source to most recent anomaly for that source
	bySource := make(map[string]observer.AnomalyOutput)

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

// GetBuffer returns the current buffer (for testing).
func (c *CrossSignalCorrelator) GetBuffer() []timestampedAnomaly { //nolint:revive // unexported return is acceptable for testing
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

		var eventSignalsCopy []observer.EventSignal
		if len(ac.EventSignals) > 0 {
			eventSignalsCopy = make([]observer.EventSignal, len(ac.EventSignals))
			copy(eventSignalsCopy, ac.EventSignals)
		}

		result = append(result, observer.ActiveCorrelation{
			Pattern:      ac.Pattern,
			Title:        ac.Title,
			Signals:      append([]string(nil), ac.Signals...),
			Anomalies:    anomaliesCopy,
			EventSignals: eventSignalsCopy,
			FirstSeen:    ac.FirstSeen,
			LastUpdated:  ac.LastUpdated,
		})
	}
	return result
}

// GetEventSignals returns the current event signals buffer (for testing).
func (c *CrossSignalCorrelator) GetEventSignals() []timestampedEventSignal { //nolint:revive // unexported return is acceptable for testing
	return c.eventSignals
}
