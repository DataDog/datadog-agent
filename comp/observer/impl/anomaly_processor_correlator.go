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
var knownPatterns = []correlationPattern{
	{
		name:           "kernel_bottleneck",
		requiredSources: []string{"network.retransmits", "ebpf.lock_contention_ns", "connection.errors"},
		reportTitle:    "Correlated: Kernel network bottleneck",
	},
}

// CrossSignalCorrelator clusters anomalies from different signals within a time window
// and detects known patterns.
type CrossSignalCorrelator struct {
	config CorrelatorConfig
	buffer []timestampedAnomaly
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
		config: config,
		buffer: nil,
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

// Flush checks for known patterns in the buffered anomalies and returns reports.
// The buffer is NOT cleared after flush to allow patterns to persist across flushes.
func (c *CrossSignalCorrelator) Flush() []observer.ReportOutput {
	if len(c.buffer) == 0 {
		return nil
	}

	// Evict old entries before checking patterns
	c.evictOldEntries(c.config.Now())

	// Extract unique signal sources
	sourceSet := make(map[string]struct{})
	for _, entry := range c.buffer {
		sourceSet[entry.anomaly.Source] = struct{}{}
	}

	// Check against known patterns
	var reports []observer.ReportOutput
	for _, pattern := range knownPatterns {
		if c.patternMatches(pattern, sourceSet) {
			reports = append(reports, c.buildReport(pattern, sourceSet))
		}
	}

	return reports
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
