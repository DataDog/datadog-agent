// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"
	"sort"
	"sync"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// LeadLagConfig configures the LeadLagCorrelator.
type LeadLagConfig struct {
	// MaxLagSeconds is the maximum lag to track between source pairs.
	// Default: 30 seconds
	MaxLagSeconds int64

	// MinObservations is the minimum number of lag observations before reporting an edge.
	// Default: 3
	MinObservations int

	// ConfidenceThreshold is the minimum confidence (0-1) for reporting a lead-lag edge.
	// Default: 0.6
	ConfidenceThreshold float64

	// MaxSourceTimestamps is the maximum timestamps to keep per source.
	// Default: 100
	MaxSourceTimestamps int

	// WindowSeconds is how long to keep timestamps before eviction.
	// Default: 120 seconds
	WindowSeconds int64
}

// DefaultLeadLagConfig returns a LeadLagConfig with default values.
func DefaultLeadLagConfig() LeadLagConfig {
	return LeadLagConfig{
		MaxLagSeconds:       30,
		MinObservations:     3,
		ConfidenceThreshold: 0.6,
		MaxSourceTimestamps: 100,
		WindowSeconds:       120,
	}
}

// RingBuffer is a simple circular buffer for timestamps.
type RingBuffer struct {
	data  []int64
	size  int
	head  int
	count int
}

// NewRingBuffer creates a new ring buffer with the given capacity.
func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		data: make([]int64, capacity),
		size: capacity,
	}
}

// Add adds a timestamp to the buffer.
func (r *RingBuffer) Add(ts int64) {
	r.data[r.head] = ts
	r.head = (r.head + 1) % r.size
	if r.count < r.size {
		r.count++
	}
}

// GetAll returns all timestamps in the buffer (oldest first).
func (r *RingBuffer) GetAll() []int64 {
	if r.count == 0 {
		return nil
	}
	result := make([]int64, r.count)
	start := 0
	if r.count == r.size {
		start = r.head
	}
	for i := 0; i < r.count; i++ {
		result[i] = r.data[(start+i)%r.size]
	}
	return result
}

// GetMostRecent returns the most recent timestamp, or 0 if empty.
func (r *RingBuffer) GetMostRecent() int64 {
	if r.count == 0 {
		return 0
	}
	idx := (r.head - 1 + r.size) % r.size
	return r.data[idx]
}

// LagHistogram tracks the distribution of lags between two sources.
// Bins: [-30, -20, -10, -5, 0, +5, +10, +20, +30] seconds (configurable based on maxLag)
type LagHistogram struct {
	bins              []int   // counts per bin
	binEdges          []int64 // bin edges in seconds
	totalObservations int
}

// NewLagHistogram creates a histogram with bins for the given max lag.
func NewLagHistogram(maxLag int64) *LagHistogram {
	// Create bins: negative lags (B leads A), zero, positive lags (A leads B)
	// e.g., for maxLag=30: [-30,-20,-10,-5,0,5,10,20,30]
	edges := []int64{-maxLag, -maxLag * 2 / 3, -maxLag / 3, -maxLag / 6, 0, maxLag / 6, maxLag / 3, maxLag * 2 / 3, maxLag}
	return &LagHistogram{
		bins:     make([]int, len(edges)-1),
		binEdges: edges,
	}
}

// Add records a lag observation.
func (h *LagHistogram) Add(lagSeconds int64) {
	h.totalObservations++
	// Find the right bin
	for i := 0; i < len(h.bins); i++ {
		if lagSeconds >= h.binEdges[i] && lagSeconds < h.binEdges[i+1] {
			h.bins[i]++
			return
		}
	}
	// Handle edge case: exactly at max
	if lagSeconds >= h.binEdges[len(h.binEdges)-1] {
		h.bins[len(h.bins)-1]++
	} else if lagSeconds < h.binEdges[0] {
		h.bins[0]++
	}
}

// Analyze returns the dominant lag direction, typical lag, and confidence.
func (h *LagHistogram) Analyze() (leader string, typicalLag int64, confidence float64) {
	if h.totalObservations == 0 {
		return "", 0, 0
	}

	// Count observations in negative bins (B leads A) vs positive bins (A leads B)
	negCount := 0
	posCount := 0
	midBin := len(h.bins) / 2

	for i := 0; i < midBin; i++ {
		negCount += h.bins[i]
	}
	for i := midBin + 1; i < len(h.bins); i++ {
		posCount += h.bins[i]
	}
	zeroCount := h.bins[midBin]

	// Determine direction
	total := float64(h.totalObservations)
	if posCount > negCount && posCount > zeroCount {
		// A leads B (positive lags dominate)
		confidence = float64(posCount) / total
		// Find weighted average of positive bins
		sum := int64(0)
		count := 0
		for i := midBin + 1; i < len(h.bins); i++ {
			binCenter := (h.binEdges[i] + h.binEdges[i+1]) / 2
			sum += binCenter * int64(h.bins[i])
			count += h.bins[i]
		}
		if count > 0 {
			typicalLag = sum / int64(count)
		}
		return "A", typicalLag, confidence
	} else if negCount > posCount && negCount > zeroCount {
		// B leads A (negative lags dominate)
		confidence = float64(negCount) / total
		sum := int64(0)
		count := 0
		for i := 0; i < midBin; i++ {
			binCenter := (h.binEdges[i] + h.binEdges[i+1]) / 2
			sum += binCenter * int64(h.bins[i])
			count += h.bins[i]
		}
		if count > 0 {
			typicalLag = -sum / int64(count) // Make positive for reporting
		}
		return "B", typicalLag, confidence
	}

	// No clear direction
	return "", 0, float64(zeroCount) / total
}

// LeadLagEdge represents a detected lead-lag relationship between two sources.
type LeadLagEdge struct {
	Leader       string  `json:"leader"`      // Source that leads
	Follower     string  `json:"follower"`    // Source that follows
	TypicalLag   int64   `json:"typical_lag"` // Seconds
	Confidence   float64 `json:"confidence"`  // 0-1
	Observations int     `json:"observations"`
}

// LeadLagCorrelator detects temporal lead-lag relationships between anomaly sources.
// It tracks when source A's anomalies consistently precede source B's anomalies,
// suggesting A may be a root cause of B.
type LeadLagCorrelator struct {
	config LeadLagConfig

	// Recent anomaly timestamps per source
	sourceTimestamps map[string]*RingBuffer

	// Lag histograms for source pairs: "A|B" -> histogram of (B_time - A_time)
	lagHistograms map[string]*LagHistogram

	// Recent anomalies for reporting
	recentAnomalies []observer.AnomalyOutput

	// Current data time for eviction
	currentDataTime int64

	mu sync.RWMutex
}

// NewLeadLagCorrelator creates a new LeadLagCorrelator with the given config.
func NewLeadLagCorrelator(config LeadLagConfig) *LeadLagCorrelator {
	if config.MaxLagSeconds == 0 {
		config.MaxLagSeconds = 30
	}
	if config.MinObservations == 0 {
		config.MinObservations = 3
	}
	if config.ConfidenceThreshold == 0 {
		config.ConfidenceThreshold = 0.6
	}
	if config.MaxSourceTimestamps == 0 {
		config.MaxSourceTimestamps = 100
	}
	if config.WindowSeconds == 0 {
		config.WindowSeconds = 120
	}
	return &LeadLagCorrelator{
		config:           config,
		sourceTimestamps: make(map[string]*RingBuffer),
		lagHistograms:    make(map[string]*LagHistogram),
	}
}

// Name returns the processor name.
func (c *LeadLagCorrelator) Name() string {
	return "lead_lag_correlator"
}

// Process adds an anomaly and updates lag histograms for all source pairs.
func (c *LeadLagCorrelator) Process(anomaly observer.AnomalyOutput) {
	c.mu.Lock()
	defer c.mu.Unlock()

	source := anomaly.Source
	ts := anomaly.Timestamp

	// Update current data time
	if ts > c.currentDataTime {
		c.currentDataTime = ts
	}

	// Store anomaly for reporting
	c.recentAnomalies = append(c.recentAnomalies, anomaly)

	// Get or create ring buffer for this source
	if _, ok := c.sourceTimestamps[source]; !ok {
		c.sourceTimestamps[source] = NewRingBuffer(c.config.MaxSourceTimestamps)
	}

	// For each other source with recent timestamps, update lag histogram
	for otherSource, otherBuffer := range c.sourceTimestamps {
		if otherSource == source {
			continue
		}

		// Get most recent timestamp from other source
		otherTs := otherBuffer.GetMostRecent()
		if otherTs == 0 {
			continue
		}

		// Only consider if within max lag window
		lag := ts - otherTs
		if lag < -c.config.MaxLagSeconds || lag > c.config.MaxLagSeconds {
			continue
		}

		// Update histogram for pair (source, otherSource)
		// Positive lag means: otherSource happened first, then source happened
		pairKey := c.pairKey(otherSource, source) // consistent ordering
		if _, ok := c.lagHistograms[pairKey]; !ok {
			c.lagHistograms[pairKey] = NewLagHistogram(c.config.MaxLagSeconds)
		}

		// Determine which direction to record
		if otherSource < source {
			// pairKey is "otherSource|source", lag is (source_time - otherSource_time)
			c.lagHistograms[pairKey].Add(lag)
		} else {
			// pairKey is "source|otherSource", lag should be (otherSource_time - source_time)
			c.lagHistograms[pairKey].Add(-lag)
		}
	}

	// Add timestamp for this source
	c.sourceTimestamps[source].Add(ts)
}

// pairKey returns a consistent key for a source pair (alphabetically ordered).
func (c *LeadLagCorrelator) pairKey(a, b string) string {
	if a < b {
		return a + "|" + b
	}
	return b + "|" + a
}

// Flush evicts old data and returns empty (reporters pull state via ActiveCorrelations).
func (c *LeadLagCorrelator) Flush() []observer.ReportOutput {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict old anomalies
	cutoff := c.currentDataTime - c.config.WindowSeconds
	newAnomalies := c.recentAnomalies[:0]
	for _, a := range c.recentAnomalies {
		if a.Timestamp >= cutoff {
			newAnomalies = append(newAnomalies, a)
		}
	}
	c.recentAnomalies = newAnomalies

	return nil
}

// GetEdges returns all detected lead-lag edges that meet the confidence threshold.
func (c *LeadLagCorrelator) GetEdges() []LeadLagEdge {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var edges []LeadLagEdge

	for pairKey, histogram := range c.lagHistograms {
		if histogram.totalObservations < c.config.MinObservations {
			continue
		}

		// Parse pair key
		var sourceA, sourceB string
		for i, ch := range pairKey {
			if ch == '|' {
				sourceA = pairKey[:i]
				sourceB = pairKey[i+1:]
				break
			}
		}

		leader, typicalLag, confidence := histogram.Analyze()
		if confidence < c.config.ConfidenceThreshold {
			continue
		}

		var leaderSource, followerSource string
		if leader == "A" {
			leaderSource = sourceA
			followerSource = sourceB
		} else if leader == "B" {
			leaderSource = sourceB
			followerSource = sourceA
		} else {
			continue // No clear direction
		}

		edges = append(edges, LeadLagEdge{
			Leader:       leaderSource,
			Follower:     followerSource,
			TypicalLag:   int64(math.Abs(float64(typicalLag))),
			Confidence:   confidence,
			Observations: histogram.totalObservations,
		})
	}

	// Sort by confidence (highest first), then by observations
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Confidence != edges[j].Confidence {
			return edges[i].Confidence > edges[j].Confidence
		}
		return edges[i].Observations > edges[j].Observations
	})

	return edges
}

// ActiveCorrelations returns lead-lag patterns as correlations for reporting.
// Implements CorrelationState interface.
func (c *LeadLagCorrelator) ActiveCorrelations() []observer.ActiveCorrelation {
	edges := c.GetEdges()

	c.mu.RLock()
	defer c.mu.RUnlock()

	var result []observer.ActiveCorrelation

	// Group anomalies by source for quick lookup
	anomaliesBySource := make(map[string][]observer.AnomalyOutput)
	for _, a := range c.recentAnomalies {
		anomaliesBySource[a.Source] = append(anomaliesBySource[a.Source], a)
	}

	// Create a correlation for each significant lead-lag chain
	for _, edge := range edges {
		sources := []string{edge.Leader, edge.Follower}

		// Collect anomalies from both sources
		var anomalies []observer.AnomalyOutput
		anomalies = append(anomalies, anomaliesBySource[edge.Leader]...)
		anomalies = append(anomalies, anomaliesBySource[edge.Follower]...)

		// Find time range
		var firstSeen, lastUpdated int64
		for _, a := range anomalies {
			if firstSeen == 0 || a.Timestamp < firstSeen {
				firstSeen = a.Timestamp
			}
			if a.Timestamp > lastUpdated {
				lastUpdated = a.Timestamp
			}
		}

		result = append(result, observer.ActiveCorrelation{
			Pattern: fmt.Sprintf("lead_lag_%s_to_%s", edge.Leader, edge.Follower),
			Title: fmt.Sprintf("Temporal: %s leads %s by ~%ds (%.0f%% confidence, %d observations)",
				edge.Leader, edge.Follower, edge.TypicalLag, edge.Confidence*100, edge.Observations),
			SourceNames: sources,
			Anomalies:   anomalies,
			FirstSeen:   firstSeen,
			LastUpdated: lastUpdated,
		})
	}

	return result
}

// GetStats returns statistics about the correlator state.
func (c *LeadLagCorrelator) GetStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return map[string]interface{}{
		"sources_tracked":      len(c.sourceTimestamps),
		"pairs_tracked":        len(c.lagHistograms),
		"edges_detected":       len(c.GetEdges()),
		"recent_anomalies":     len(c.recentAnomalies),
		"max_lag_seconds":      c.config.MaxLagSeconds,
		"min_observations":     c.config.MinObservations,
		"confidence_threshold": c.config.ConfidenceThreshold,
		"current_data_time":    c.currentDataTime,
	}
}
