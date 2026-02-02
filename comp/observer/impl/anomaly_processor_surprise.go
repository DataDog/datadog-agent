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

// SurpriseConfig configures the SurpriseCorrelator.
type SurpriseConfig struct {
	// WindowSizeSeconds is the time window for co-occurrence detection.
	// Sources with anomalies in the same window are considered co-occurring.
	// Default: 10 seconds
	WindowSizeSeconds int64

	// MinLift is the minimum lift value to report as a surprising co-occurrence.
	// lift > 1 means sources co-occur more than expected by chance.
	// Default: 2.0
	MinLift float64

	// MaxLift is the maximum lift value to report as an expected-but-rare pattern.
	// lift < 1 means sources co-occur less than expected by chance.
	// Default: 0.5
	MaxLift float64

	// MinSupport is the minimum number of co-occurrences to report.
	// Default: 2
	MinSupport int

	// MinSourceCount is the minimum number of anomalies a source must have to be considered.
	// Default: 2
	MinSourceCount int

	// MaxPairsTracked is the maximum number of pairs to track to limit memory.
	// Default: 10000
	MaxPairsTracked int

	// EvictionWindowSeconds is how long to keep data before eviction.
	// Default: 300 seconds (5 minutes)
	EvictionWindowSeconds int64
}

// DefaultSurpriseConfig returns a SurpriseConfig with default values.
func DefaultSurpriseConfig() SurpriseConfig {
	return SurpriseConfig{
		WindowSizeSeconds:     10,
		MinLift:               2.0,
		MaxLift:               0.5,
		MinSupport:            2,
		MinSourceCount:        2,
		MaxPairsTracked:       10000,
		EvictionWindowSeconds: 300,
	}
}

// SurpriseEdge represents a surprising (or unexpectedly rare) co-occurrence.
type SurpriseEdge struct {
	Source1      string  `json:"source1"`
	Source2      string  `json:"source2"`
	Lift         float64 `json:"lift"`          // > 1 = surprising co-occurrence, < 1 = surprisingly rare
	Support      int     `json:"support"`       // Number of co-occurrences
	Source1Count int     `json:"source1_count"` // Total anomalies from source1
	Source2Count int     `json:"source2_count"` // Total anomalies from source2
	IsSurprising bool    `json:"is_surprising"` // true if lift > MinLift, false if lift < MaxLift
}

// SurpriseCorrelator detects unexpected co-occurrences using lift metric.
// Lift measures how much more (or less) often two sources co-occur than expected by chance:
//
//	lift(A,B) = P(A ∩ B) / (P(A) × P(B))
//	          = (pairCount[A,B] / totalWindows) / ((sourceCount[A] / totalWindows) × (sourceCount[B] / totalWindows))
//	          = pairCount[A,B] × totalWindows / (sourceCount[A] × sourceCount[B])
//
// - lift > 2.0: A and B co-occur MORE than expected → interesting pattern
// - lift < 0.5: A and B co-occur LESS than expected → also interesting (anti-correlation)
// - lift ≈ 1.0: Independent, just random co-occurrence
type SurpriseCorrelator struct {
	config SurpriseConfig

	// Marginal counts per source (how often each source has anomalies)
	sourceCounts map[string]int

	// Joint counts for pairs (how often A and B co-occur in the same window)
	pairCounts map[string]int // "A|B" -> count

	// Total time windows observed
	totalWindows int

	// Current window tracking
	currentWindowStart   int64
	currentWindowSources map[string]bool

	// Recent anomalies for reporting
	recentAnomalies []observer.AnomalyOutput

	// Current data time for eviction
	currentDataTime int64

	mu sync.RWMutex
}

// NewSurpriseCorrelator creates a new SurpriseCorrelator with the given config.
func NewSurpriseCorrelator(config SurpriseConfig) *SurpriseCorrelator {
	if config.WindowSizeSeconds == 0 {
		config.WindowSizeSeconds = 10
	}
	if config.MinLift == 0 {
		config.MinLift = 2.0
	}
	if config.MaxLift == 0 {
		config.MaxLift = 0.5
	}
	if config.MinSupport == 0 {
		config.MinSupport = 2
	}
	if config.MinSourceCount == 0 {
		config.MinSourceCount = 2
	}
	if config.MaxPairsTracked == 0 {
		config.MaxPairsTracked = 10000
	}
	if config.EvictionWindowSeconds == 0 {
		config.EvictionWindowSeconds = 300
	}
	return &SurpriseCorrelator{
		config:               config,
		sourceCounts:         make(map[string]int),
		pairCounts:           make(map[string]int),
		currentWindowSources: make(map[string]bool),
	}
}

// Name returns the processor name.
func (c *SurpriseCorrelator) Name() string {
	return "surprise_correlator"
}

// Process adds an anomaly and updates co-occurrence counts.
func (c *SurpriseCorrelator) Process(anomaly observer.AnomalyOutput) {
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

	// Determine which window this anomaly belongs to
	windowStart := (ts / c.config.WindowSizeSeconds) * c.config.WindowSizeSeconds

	// If we've moved to a new window, finalize the previous one
	if windowStart > c.currentWindowStart {
		c.finalizeWindow()
		c.currentWindowStart = windowStart
		c.currentWindowSources = make(map[string]bool)
	}

	// Add this source to the current window
	c.currentWindowSources[source] = true
}

// finalizeWindow processes the completed window and updates counts.
func (c *SurpriseCorrelator) finalizeWindow() {
	if len(c.currentWindowSources) == 0 {
		return
	}

	c.totalWindows++

	// Update source counts
	for source := range c.currentWindowSources {
		c.sourceCounts[source]++
	}

	// Update pair counts for all co-occurring sources
	sources := make([]string, 0, len(c.currentWindowSources))
	for source := range c.currentWindowSources {
		sources = append(sources, source)
	}
	sort.Strings(sources) // Ensure consistent ordering

	// Track all pairs (limit to avoid memory explosion)
	if len(c.pairCounts) < c.config.MaxPairsTracked {
		for i := 0; i < len(sources); i++ {
			for j := i + 1; j < len(sources); j++ {
				pairKey := sources[i] + "|" + sources[j]
				c.pairCounts[pairKey]++
			}
		}
	}
}

// Flush evicts old data and returns empty (reporters pull state via ActiveCorrelations).
func (c *SurpriseCorrelator) Flush() []observer.ReportOutput {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Finalize current window if it has data
	c.finalizeWindow()

	// Evict old anomalies
	cutoff := c.currentDataTime - c.config.EvictionWindowSeconds
	newAnomalies := c.recentAnomalies[:0]
	for _, a := range c.recentAnomalies {
		if a.Timestamp >= cutoff {
			newAnomalies = append(newAnomalies, a)
		}
	}
	c.recentAnomalies = newAnomalies

	return nil
}

// GetEdges returns all detected surprise edges (both high and low lift).
func (c *SurpriseCorrelator) GetEdges() []SurpriseEdge {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.totalWindows == 0 {
		return nil
	}

	var edges []SurpriseEdge

	for pairKey, pairCount := range c.pairCounts {
		if pairCount < c.config.MinSupport {
			continue
		}

		// Parse pair key
		var source1, source2 string
		for i, ch := range pairKey {
			if ch == '|' {
				source1 = pairKey[:i]
				source2 = pairKey[i+1:]
				break
			}
		}

		count1, ok1 := c.sourceCounts[source1]
		count2, ok2 := c.sourceCounts[source2]
		if !ok1 || !ok2 {
			continue
		}

		// Skip sources with too few anomalies
		if count1 < c.config.MinSourceCount || count2 < c.config.MinSourceCount {
			continue
		}

		// Calculate lift
		// lift = P(A ∩ B) / (P(A) × P(B))
		//      = (pairCount / totalWindows) / ((count1 / totalWindows) × (count2 / totalWindows))
		//      = pairCount × totalWindows / (count1 × count2)
		lift := float64(pairCount) * float64(c.totalWindows) / (float64(count1) * float64(count2))

		// Only include if surprising (high or low lift)
		isSurprising := lift >= c.config.MinLift
		isRare := lift <= c.config.MaxLift

		if !isSurprising && !isRare {
			continue
		}

		edges = append(edges, SurpriseEdge{
			Source1:      source1,
			Source2:      source2,
			Lift:         lift,
			Support:      pairCount,
			Source1Count: count1,
			Source2Count: count2,
			IsSurprising: isSurprising,
		})
	}

	// Sort by lift (highest first for surprising, lowest first for rare)
	sort.Slice(edges, func(i, j int) bool {
		// Surprising patterns first, sorted by lift descending
		if edges[i].IsSurprising && !edges[j].IsSurprising {
			return true
		}
		if !edges[i].IsSurprising && edges[j].IsSurprising {
			return false
		}
		if edges[i].IsSurprising {
			return edges[i].Lift > edges[j].Lift
		}
		// Rare patterns sorted by lift ascending
		return edges[i].Lift < edges[j].Lift
	})

	return edges
}

// ActiveCorrelations returns surprise patterns as correlations for reporting.
// Implements CorrelationState interface.
func (c *SurpriseCorrelator) ActiveCorrelations() []observer.ActiveCorrelation {
	edges := c.GetEdges()

	c.mu.RLock()
	defer c.mu.RUnlock()

	var result []observer.ActiveCorrelation

	// Group anomalies by source for quick lookup
	anomaliesBySource := make(map[string][]observer.AnomalyOutput)
	for _, a := range c.recentAnomalies {
		anomaliesBySource[a.Source] = append(anomaliesBySource[a.Source], a)
	}

	// Create a correlation for each significant surprise pattern
	for _, edge := range edges {
		sources := []string{edge.Source1, edge.Source2}

		// Collect anomalies from both sources
		var anomalies []observer.AnomalyOutput
		anomalies = append(anomalies, anomaliesBySource[edge.Source1]...)
		anomalies = append(anomalies, anomaliesBySource[edge.Source2]...)

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

		var title string
		var pattern string
		if edge.IsSurprising {
			title = fmt.Sprintf("Surprising co-occurrence: %s + %s (lift=%.1f, %d times)",
				edge.Source1, edge.Source2, edge.Lift, edge.Support)
			pattern = fmt.Sprintf("surprise_%s_and_%s", edge.Source1, edge.Source2)
		} else {
			title = fmt.Sprintf("Unexpectedly rare: %s + %s usually co-occur but didn't (lift=%.2f)",
				edge.Source1, edge.Source2, edge.Lift)
			pattern = fmt.Sprintf("missing_%s_and_%s", edge.Source1, edge.Source2)
		}

		result = append(result, observer.ActiveCorrelation{
			Pattern:     pattern,
			Title:       title,
			Sources:     sources,
			Anomalies:   anomalies,
			FirstSeen:   firstSeen,
			LastUpdated: lastUpdated,
		})
	}

	return result
}

// GetStats returns statistics about the correlator state.
func (c *SurpriseCorrelator) GetStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	surprisingCount := 0
	rareCount := 0
	for _, edge := range c.GetEdges() {
		if edge.IsSurprising {
			surprisingCount++
		} else {
			rareCount++
		}
	}

	return map[string]interface{}{
		"sources_tracked":        len(c.sourceCounts),
		"pairs_tracked":          len(c.pairCounts),
		"total_windows":          c.totalWindows,
		"surprising_patterns":    surprisingCount,
		"rare_patterns":          rareCount,
		"recent_anomalies":       len(c.recentAnomalies),
		"window_size_seconds":    c.config.WindowSizeSeconds,
		"min_lift":               c.config.MinLift,
		"max_lift":               c.config.MaxLift,
		"min_support":            c.config.MinSupport,
		"current_window_start":   c.currentWindowStart,
		"current_window_sources": len(c.currentWindowSources),
		"current_data_time":      c.currentDataTime,
	}
}
