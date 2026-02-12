// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"sync"
	"time"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// GraphSketchCorrelatorConfig configures the GraphSketch-based anomaly correlator.
type GraphSketchCorrelatorConfig struct {
	// Count-Min Sketch dimensions
	Depth int // Number of hash functions (rows), default: 4
	Width int // Number of buckets per hash (columns), default: 256

	// Temporal binning
	NumTimeBins int   // Number of temporal layers, default: 6
	TimeBinSize int64 // Duration of each time bin in seconds, default: 30

	// Co-occurrence window (same as time slack)
	CoOccurrenceWindow int64 // Seconds within which anomalies are considered co-occurring, default: 10

	// Minimum correlation strength to group
	MinCorrelationStrength float64 // Minimum edge frequency to consider correlated, default: 2.0

	// Cluster settings
	MinClusterSize int   // Minimum anomalies to form a reportable cluster, default: 2
	WindowSeconds  int64 // How long to keep anomalies before eviction, default: 60

	// Decay for older bins
	DecayFactor float64 // Exponential decay for older bins, default: 0.85
}

// DefaultGraphSketchCorrelatorConfig returns a config with sensible defaults.
func DefaultGraphSketchCorrelatorConfig() GraphSketchCorrelatorConfig {
	return GraphSketchCorrelatorConfig{
		Depth:                  4,
		Width:                  256,
		NumTimeBins:            6,
		TimeBinSize:            30,
		CoOccurrenceWindow:     10,
		MinCorrelationStrength: 2.0,
		MinClusterSize:         2,
		WindowSeconds:          60,
		DecayFactor:            0.85,
	}
}

// GraphSketchCorrelator groups anomalies based on learned co-occurrence patterns.
// It implements AnomalyProcessor and CorrelationState interfaces.
type GraphSketchCorrelator struct {
	config GraphSketchCorrelatorConfig
	mu     sync.RWMutex // Protects concurrent access

	// 3D Tensor Sketch: [time_bin][depth][width]
	tensorSketch [][][]float64

	// Time bin tracking
	currentTimeBin int64
	layerToBinMap  map[int]int64

	// Anomaly buffer for co-occurrence detection
	anomalyBuffer []observer.AnomalyOutput

	// Clusters based on co-occurrence
	clusters        []*graphCluster
	nextClusterID   int
	currentDataTime int64

	// Edge frequency tracking (for quick lookups and reporting)
	knownEdges    map[seriesPairKey]int   // edge -> observation count
	edgeFirstSeen map[seriesPairKey]int64 // edge -> unix timestamp when first seen

	// Stability tracking
	lastNewEdgeTime   int64 // timestamp when last NEW edge was discovered
	totalObservations int   // total number of edge observations processed
	lastProcessTime   int64 // wall clock time when Process() was last called
	frozen            bool  // true = no more data expected, stop updating

	// Progress tracking for correlation algorithm
	uniqueSources map[observer.SeriesID]bool // unique anomaly sources seen

	// Caching for expensive edge ranking (fixes backpressure)
	cachedTopEdges      []EdgeInfo
	cacheValidUntilData int64 // data timestamp when cache expires (uses data time, not wall clock)
	cacheTTLDataSeconds int64 // how long cache is valid in data-time seconds (default: 5s)
}

// EdgeInfo stores metadata about a learned edge.
type EdgeInfo struct {
	Source1       string
	Source2       string
	EdgeKey       string
	Observations  int     // Raw count of observations
	Frequency     float64 // Decay-weighted frequency
	FirstSeenUnix int64   // Unix timestamp when this edge was first observed
}

// graphCluster represents a group of anomalies with strong co-occurrence.
type graphCluster struct {
	id        int
	anomalies map[observer.SeriesID]observer.AnomalyOutput // keyed by SourceSeriesID for dedup
	timeRange observer.TimeRange
	strength  float64 // Average edge strength within cluster
}

// NewGraphSketchCorrelator creates a new GraphSketch-based correlator.
func NewGraphSketchCorrelator(config GraphSketchCorrelatorConfig) *GraphSketchCorrelator {
	if config.Depth == 0 {
		config.Depth = 4
	}
	if config.Width == 0 {
		config.Width = 256
	}
	if config.NumTimeBins == 0 {
		config.NumTimeBins = 6
	}
	if config.TimeBinSize == 0 {
		config.TimeBinSize = 30
	}
	if config.CoOccurrenceWindow == 0 {
		config.CoOccurrenceWindow = 10
	}
	if config.MinCorrelationStrength == 0 {
		config.MinCorrelationStrength = 2.0
	}
	if config.MinClusterSize == 0 {
		config.MinClusterSize = 2
	}
	if config.WindowSeconds == 0 {
		config.WindowSeconds = 60
	}
	if config.DecayFactor == 0 {
		config.DecayFactor = 0.85
	}

	// Initialize 3D tensor sketch
	tensorSketch := make([][][]float64, config.NumTimeBins)
	for t := 0; t < config.NumTimeBins; t++ {
		tensorSketch[t] = make([][]float64, config.Depth)
		for d := 0; d < config.Depth; d++ {
			tensorSketch[t][d] = make([]float64, config.Width)
		}
	}

	return &GraphSketchCorrelator{
		config:              config,
		tensorSketch:        tensorSketch,
		layerToBinMap:       make(map[int]int64),
		anomalyBuffer:       make([]observer.AnomalyOutput, 0, 100),
		clusters:            nil,
		edgeFirstSeen:       make(map[seriesPairKey]int64),
		knownEdges:          make(map[seriesPairKey]int),
		uniqueSources:       make(map[observer.SeriesID]bool),
		cacheTTLDataSeconds: 5, // Cache edge rankings for 5 data-time seconds to reduce backpressure
	}
}

// Name returns the processor name.
func (g *GraphSketchCorrelator) Name() string {
	return "graphsketch_correlator"
}

// Freeze locks the correlator state - no more updates will be made.
// Call this when data replay is complete to prevent further decay/eviction.
func (g *GraphSketchCorrelator) Freeze() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.frozen = true
	// Invalidate cache so final state is reflected immediately
	g.cacheValidUntilData = 0
	g.cachedTopEdges = nil
	fmt.Println("[GraphSketch] Correlator frozen - state is now final")
}

// IsFrozen returns true if the correlator is frozen.
func (g *GraphSketchCorrelator) IsFrozen() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.frozen
}

// getAnomalyTime returns the effective timestamp for an anomaly.
// Prefers TimeRange.End, falls back to Timestamp if TimeRange is not set.
func getAnomalyTime(anomaly observer.AnomalyOutput) int64 {
	if anomaly.TimeRange.End > 0 {
		return anomaly.TimeRange.End
	}
	return anomaly.Timestamp
}

// Process adds an anomaly, updating co-occurrence tracking and clustering.
func (g *GraphSketchCorrelator) Process(anomaly observer.AnomalyOutput) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.frozen {
		return // Do not process if frozen
	}

	// Track wall clock time of last Process call
	g.lastProcessTime = time.Now().Unix()

	// Get anomaly timestamp
	anomalyTime := getAnomalyTime(anomaly)

	// Update current data time
	if anomalyTime > g.currentDataTime {
		g.currentDataTime = anomalyTime
	}

	// Add to buffer
	g.anomalyBuffer = append(g.anomalyBuffer, anomaly)

	// Track unique sources
	g.uniqueSources[anomaly.SourceSeriesID] = true

	// Find co-occurring anomalies
	coOccurring := g.findCoOccurring(anomaly)

	// Update tensor sketch for each co-occurring pair
	timeBin := anomalyTime / g.config.TimeBinSize
	g.advanceTimeBin(timeBin)

	for _, other := range coOccurring {
		if anomaly.SourceSeriesID == other.SourceSeriesID {
			continue
		}
		edge := g.canonicalEdge(anomaly.SourceSeriesID, other.SourceSeriesID)
		g.updateTensorSketch(edge, timeBin)

		// Track first seen - this is a NEW edge
		if _, exists := g.edgeFirstSeen[edge]; !exists {
			g.edgeFirstSeen[edge] = anomalyTime
			g.lastNewEdgeTime = anomalyTime // Record when we last discovered something new
		}

		// Track edge observations for observability
		g.knownEdges[edge]++
		g.totalObservations++
	}

	// Find or create cluster for this anomaly
	g.clusterAnomaly(anomaly)

	// Prune old anomalies from buffer
	g.pruneBufferLocked()
}

// findCoOccurring finds anomalies within the co-occurrence window.
func (g *GraphSketchCorrelator) findCoOccurring(anomaly observer.AnomalyOutput) []observer.AnomalyOutput {
	var coOccurring []observer.AnomalyOutput
	window := g.config.CoOccurrenceWindow
	anomalyTime := getAnomalyTime(anomaly)

	for _, other := range g.anomalyBuffer {
		// Check if within co-occurrence window
		otherTime := getAnomalyTime(other)
		if math.Abs(float64(anomalyTime-otherTime)) <= float64(window) {
			coOccurring = append(coOccurring, other)
		}
	}
	return coOccurring
}

// pruneBufferLocked removes anomalies older than the window from the buffer.
func (g *GraphSketchCorrelator) pruneBufferLocked() {
	if len(g.anomalyBuffer) == 0 {
		return
	}

	cutoff := g.currentDataTime - g.config.WindowSeconds
	newBuffer := make([]observer.AnomalyOutput, 0, len(g.anomalyBuffer))
	for _, anomaly := range g.anomalyBuffer {
		if getAnomalyTime(anomaly) >= cutoff {
			newBuffer = append(newBuffer, anomaly)
		}
	}
	g.anomalyBuffer = newBuffer
}

// canonicalEdge returns a canonical key for an edge.
func (g *GraphSketchCorrelator) canonicalEdge(s1, s2 observer.SeriesID) seriesPairKey {
	return newSeriesPairKey(s1, s2)
}

// advanceTimeBin advances to a new time bin, managing the ring buffer.
func (g *GraphSketchCorrelator) advanceTimeBin(newTimeBin int64) {
	if newTimeBin <= g.currentTimeBin {
		return
	}

	for b := g.currentTimeBin + 1; b <= newTimeBin; b++ {
		layer := int(b % int64(g.config.NumTimeBins))

		if oldBin, exists := g.layerToBinMap[layer]; exists {
			if b-oldBin >= int64(g.config.NumTimeBins) {
				g.clearLayer(layer)
			}
		}

		g.layerToBinMap[layer] = b
	}

	g.currentTimeBin = newTimeBin
}

// clearLayer zeros all counters in a tensor layer.
func (g *GraphSketchCorrelator) clearLayer(layer int) {
	for d := 0; d < g.config.Depth; d++ {
		for w := 0; w < g.config.Width; w++ {
			g.tensorSketch[layer][d][w] = 0.0
		}
	}
}

// updateTensorSketch updates the tensor with an edge observation.
func (g *GraphSketchCorrelator) updateTensorSketch(edge seriesPairKey, timeBin int64) {
	layer := int(timeBin % int64(g.config.NumTimeBins))
	hashKey := edge.hashKey()

	// Conservative update: increment only buckets at minimum
	minCount := math.MaxFloat64
	for d := 0; d < g.config.Depth; d++ {
		idx := g.hash(hashKey, d)
		if g.tensorSketch[layer][d][idx] < minCount {
			minCount = g.tensorSketch[layer][d][idx]
		}
	}

	for d := 0; d < g.config.Depth; d++ {
		idx := g.hash(hashKey, d)
		if g.tensorSketch[layer][d][idx] == minCount {
			g.tensorSketch[layer][d][idx]++
		}
	}
}

// queryEdgeFrequency queries the cumulative frequency for an edge.
func (g *GraphSketchCorrelator) queryEdgeFrequency(edge seriesPairKey) float64 {
	var cumulativeFreq float64
	gamma := g.config.DecayFactor
	hashKey := edge.hashKey()

	for layer := 0; layer < g.config.NumTimeBins; layer++ {
		if absoluteBin, exists := g.layerToBinMap[layer]; exists {
			if absoluteBin <= g.currentTimeBin {
				estimates := make([]float64, g.config.Depth)
				for d := 0; d < g.config.Depth; d++ {
					idx := g.hash(hashKey, d)
					estimates[d] = g.tensorSketch[layer][d][idx]
				}

				minEst := estimates[0]
				for _, e := range estimates[1:] {
					if e < minEst {
						minEst = e
					}
				}

				age := g.currentTimeBin - absoluteBin
				decayWeight := math.Pow(gamma, float64(age))
				cumulativeFreq += minEst * decayWeight
			}
		}
	}

	return cumulativeFreq
}

// hash generates a hash index for an edge key.
func (g *GraphSketchCorrelator) hash(key string, hashIdx int) int {
	h := fnv.New64a()
	h.Write([]byte(key))
	h.Write([]byte{byte(hashIdx)})
	return int(h.Sum64() % uint64(g.config.Width))
}

// clusterAnomaly adds an anomaly to the appropriate cluster based on co-occurrence.
func (g *GraphSketchCorrelator) clusterAnomaly(anomaly observer.AnomalyOutput) {
	// Find existing clusters this anomaly could join
	var candidateClusters []*graphCluster
	for _, cluster := range g.clusters {
		for sourceInCluster := range cluster.anomalies {
			edge := g.canonicalEdge(anomaly.SourceSeriesID, sourceInCluster)
			if g.queryEdgeFrequency(edge) >= g.config.MinCorrelationStrength {
				candidateClusters = append(candidateClusters, cluster)
				break
			}
		}
	}

	if len(candidateClusters) > 0 {
		// Join the first candidate cluster
		g.addToCluster(candidateClusters[0], anomaly)
	} else {
		// Create a new cluster with timestamp from anomaly
		anomalyTime := getAnomalyTime(anomaly)
		newCluster := &graphCluster{
			id:        g.nextClusterID,
			anomalies: make(map[observer.SeriesID]observer.AnomalyOutput),
			timeRange: observer.TimeRange{Start: anomalyTime, End: anomalyTime},
		}
		g.nextClusterID++
		g.addToCluster(newCluster, anomaly)
		g.clusters = append(g.clusters, newCluster)
	}
}

// addToCluster adds an anomaly to a cluster and updates its time range.
// Deduplicates by source, keeping the most recent anomaly.
func (g *GraphSketchCorrelator) addToCluster(cluster *graphCluster, anomaly observer.AnomalyOutput) {
	anomalyTime := getAnomalyTime(anomaly)
	if existing, exists := cluster.anomalies[anomaly.SourceSeriesID]; exists {
		// Keep the more recent anomaly from the same series
		if anomalyTime <= getAnomalyTime(existing) {
			return
		}
	}
	cluster.anomalies[anomaly.SourceSeriesID] = anomaly
	if anomalyTime < cluster.timeRange.Start || cluster.timeRange.Start == 0 {
		cluster.timeRange.Start = anomalyTime
	}
	if anomalyTime > cluster.timeRange.End {
		cluster.timeRange.End = anomalyTime
	}
	g.updateClusterStrength(cluster)
}

// updateClusterStrength recalculates the average edge strength within a cluster.
func (g *GraphSketchCorrelator) updateClusterStrength(cluster *graphCluster) {
	if len(cluster.anomalies) < 2 {
		cluster.strength = 0
		return
	}

	var totalStrength float64
	var edgeCount int
	sources := make([]observer.SeriesID, 0, len(cluster.anomalies))
	for s := range cluster.anomalies {
		sources = append(sources, s)
	}

	for i := 0; i < len(sources); i++ {
		for j := i + 1; j < len(sources); j++ {
			edge := g.canonicalEdge(sources[i], sources[j])
			totalStrength += g.queryEdgeFrequency(edge)
			edgeCount++
		}
	}
	if edgeCount > 0 {
		cluster.strength = totalStrength / float64(edgeCount)
	} else {
		cluster.strength = 0
	}
}

// Flush evicts old clusters and returns empty (reporters pull state via ActiveCorrelations).
// Note: We do NOT apply decay here because:
// 1. Flush() is called after every metric observation (very frequently)
// 2. queryEdgeFrequency() already applies time-based decay when reading
// Applying decay in both places would cause double-decay and obliterate frequencies.
func (g *GraphSketchCorrelator) Flush() []observer.ReportOutput {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.frozen {
		return nil // Do not flush or evict if frozen
	}

	// Only evict old data, don't decay (decay is handled in queryEdgeFrequency)
	g.evictOldClustersLocked()
	g.pruneBufferLocked()
	return nil
}

// Reset clears all internal state for reanalysis.
func (g *GraphSketchCorrelator) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Reinitialize 3D tensor sketch [NumTimeBins][Depth][Width]
	g.tensorSketch = make([][][]float64, g.config.NumTimeBins)
	for t := 0; t < g.config.NumTimeBins; t++ {
		g.tensorSketch[t] = make([][]float64, g.config.Depth)
		for d := 0; d < g.config.Depth; d++ {
			g.tensorSketch[t][d] = make([]float64, g.config.Width)
		}
	}
	g.currentTimeBin = 0
	g.layerToBinMap = make(map[int]int64)
	g.anomalyBuffer = g.anomalyBuffer[:0]
	g.clusters = nil
	g.nextClusterID = 0
	g.currentDataTime = 0
	g.knownEdges = make(map[seriesPairKey]int)
	g.edgeFirstSeen = make(map[seriesPairKey]int64)
	g.uniqueSources = make(map[observer.SeriesID]bool)
	g.totalObservations = 0
	g.lastNewEdgeTime = 0
	g.cachedTopEdges = nil
	g.cacheValidUntilData = 0
	g.frozen = false
}

// evictOldClustersLocked removes clusters whose last updated time is too old.
func (g *GraphSketchCorrelator) evictOldClustersLocked() {
	if len(g.clusters) == 0 {
		return
	}

	cutoff := g.currentDataTime - g.config.WindowSeconds
	newClusters := make([]*graphCluster, 0, len(g.clusters))
	for _, cluster := range g.clusters {
		if cluster.timeRange.End >= cutoff {
			newClusters = append(newClusters, cluster)
		}
	}
	g.clusters = newClusters
}

// ActiveCorrelations returns clusters that meet the minimum size threshold.
// Implements CorrelationState interface.
func (g *GraphSketchCorrelator) ActiveCorrelations() []observer.ActiveCorrelation {
	// Try to acquire lock with timeout to avoid blocking HTTP handlers
	if !g.mu.TryRLock() {
		return nil // Return empty if lock unavailable
	}
	defer g.mu.RUnlock()

	var result []observer.ActiveCorrelation

	for _, cluster := range g.clusters {
		if len(cluster.anomalies) < g.config.MinClusterSize {
			continue
		}

		// Collect anomalies and sources
		anomalies := make([]observer.AnomalyOutput, 0, len(cluster.anomalies))
		sources := make([]observer.SeriesID, 0, len(cluster.anomalies))
		for seriesID, anomaly := range cluster.anomalies {
			anomalies = append(anomalies, anomaly)
			sources = append(sources, seriesID)
		}
		sort.Slice(sources, func(i, j int) bool { return sources[i] < sources[j] })
		metricNames := sortedUniqueMetricNames(anomalies)

		title := g.buildClusterTitle(cluster, sources)

		result = append(result, observer.ActiveCorrelation{
			Pattern:         fmt.Sprintf("graphsketch_cluster_%d", cluster.id),
			Title:           title,
			MemberSeriesIDs: sources,
			MetricNames:     metricNames,
			Anomalies:       anomalies,
			FirstSeen:       cluster.timeRange.Start,
			LastUpdated:     cluster.timeRange.End,
		})
	}

	// Sort by cluster size (largest first), then by strength
	sort.Slice(result, func(i, j int) bool {
		if len(result[i].Anomalies) != len(result[j].Anomalies) {
			return len(result[i].Anomalies) > len(result[j].Anomalies)
		}
		return result[i].FirstSeen < result[j].FirstSeen
	})

	return result
}

func (g *GraphSketchCorrelator) buildClusterTitle(cluster *graphCluster, sources []observer.SeriesID) string {
	if len(sources) == 0 {
		return fmt.Sprintf("GraphSketch Cluster %d (empty)", cluster.id)
	}
	if len(sources) == 1 {
		return fmt.Sprintf("GraphSketch Cluster %d: %s", cluster.id, string(sources[0]))
	}

	// Find the strongest edge within the cluster for the title
	var strongestEdge seriesPairKey
	var foundStrongest bool
	var maxFreq float64
	for i := 0; i < len(sources); i++ {
		for j := i + 1; j < len(sources); j++ {
			edge := g.canonicalEdge(sources[i], sources[j])
			freq := g.queryEdgeFrequency(edge)
			if freq > maxFreq {
				maxFreq = freq
				strongestEdge = edge
				foundStrongest = true
			}
		}
	}

	if foundStrongest {
		return fmt.Sprintf("GraphSketch: %s ↔ %s (%d sources)", string(strongestEdge.A), string(strongestEdge.B), len(sources))
	}

	return fmt.Sprintf("GraphSketch Cluster %d (%d sources)", cluster.id, len(sources))
}

// GetLearnedEdges returns all currently learned edges with their frequencies.
// Uses caching to avoid expensive recalculation on every call (fixes backpressure).
// Cache validity is based on data timestamps, not wall-clock time, so results
// are consistent regardless of replay speed.
func (g *GraphSketchCorrelator) GetLearnedEdges() []EdgeInfo {
	// Fast path: check cache with read lock
	g.mu.RLock()
	dataTime := g.currentDataTime
	if dataTime < g.cacheValidUntilData && g.cachedTopEdges != nil {
		edges := g.cachedTopEdges
		g.mu.RUnlock()
		return edges
	}
	g.mu.RUnlock()

	// Slow path: recompute with write lock
	g.mu.Lock()
	defer g.mu.Unlock()

	// Double-check after acquiring write lock (re-read dataTime under lock)
	dataTime = g.currentDataTime
	if dataTime < g.cacheValidUntilData && g.cachedTopEdges != nil {
		return g.cachedTopEdges
	}

	// Compute and cache
	g.cachedTopEdges = g.getLearnedEdgesLocked()
	g.cacheValidUntilData = dataTime + g.cacheTTLDataSeconds
	return g.cachedTopEdges
}

// getLearnedEdgesLocked returns edges without acquiring lock (caller must hold lock).
func (g *GraphSketchCorrelator) getLearnedEdgesLocked() []EdgeInfo {
	// First pass: collect edges with observation counts only (fast)
	type edgeCandidate struct {
		key   seriesPairKey
		count int
	}
	candidates := make([]edgeCandidate, 0, len(g.knownEdges))
	for edge, count := range g.knownEdges {
		candidates = append(candidates, edgeCandidate{key: edge, count: count})
	}

	// Sort by observation count to get top candidates
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].count > candidates[j].count
	})

	// Limit to top N candidates before expensive frequency calculation
	const maxCandidates = 200
	if len(candidates) > maxCandidates {
		candidates = candidates[:maxCandidates]
	}

	// Second pass: calculate decay-weighted frequency for top candidates
	edges := make([]EdgeInfo, 0, len(candidates))
	for _, cand := range candidates {
		edges = append(edges, EdgeInfo{
			Source1:       string(cand.key.A),
			Source2:       string(cand.key.B),
			EdgeKey:       cand.key.displayKey(),
			Observations:  cand.count,
			Frequency:     g.queryEdgeFrequency(cand.key),
			FirstSeenUnix: g.edgeFirstSeen[cand.key],
		})
	}

	// Sort by observations (highest first) - more intuitive than frequency
	sort.Slice(edges, func(i, j int) bool {
		return edges[i].Observations > edges[j].Observations
	})

	return edges
}

// GetTopEdges returns the top N edges by observations.
func (g *GraphSketchCorrelator) GetTopEdges(n int) []EdgeInfo {
	edges := g.GetLearnedEdges()
	if len(edges) > n {
		return edges[:n]
	}
	return edges
}

// GetStats returns current statistics and stability information.
func (g *GraphSketchCorrelator) GetStats() map[string]interface{} {
	// Try to acquire lock with timeout to avoid blocking HTTP handlers
	if !g.mu.TryRLock() {
		return map[string]interface{}{"status": "busy", "frozen": g.IsFrozen(), "available": true}
	}
	defer g.mu.RUnlock()

	// Calculate time since last new edge was discovered (data time)
	timeSinceNewEdge := int64(0)
	if g.lastNewEdgeTime > 0 && g.currentDataTime > 0 {
		timeSinceNewEdge = g.currentDataTime - g.lastNewEdgeTime
	}

	// Calculate time since last Process() call (wall clock) - detects when data stopped flowing
	timeSinceLastProcess := int64(0)
	if g.lastProcessTime > 0 {
		timeSinceLastProcess = time.Now().Unix() - g.lastProcessTime
	}

	// Determine stability status
	stabilityStatus := "discovering"
	stabilityPercent := 0
	statusIcon := "🔍" // Discovering

	if g.frozen {
		stabilityStatus = "complete"
		stabilityPercent = 100
		statusIcon = "🏁" // Complete
	} else if g.totalObservations == 0 {
		stabilityStatus = "discovering"
		stabilityPercent = 0
		statusIcon = "🔍" // Discovering
	} else {
		edgeCount := len(g.knownEdges)
		sourceCount := len(g.uniqueSources)

		// Progress based on unique sources discovered (up to 20%)
		if sourceCount > 0 {
			progress := int(float64(sourceCount) / 100.0 * 20)
			if progress > 20 {
				progress = 20
			}
			stabilityPercent = progress
		}

		if timeSinceNewEdge >= 60 { // No new edges for 60s (data time)
			stabilityStatus = "stable"
			stabilityPercent = 100
			statusIcon = "✅" // Stable
		} else if timeSinceNewEdge >= 30 { // No new edges for 30s (data time)
			stabilityStatus = "stabilizing"
			// Progress from 60% to 95% based on timeSinceNewEdge
			progressRange := 95 - 60
			timeProgress := float64(timeSinceNewEdge-30) / float64(60-30)
			stabilityPercent = 60 + int(timeProgress*float64(progressRange))
			if stabilityPercent > 95 {
				stabilityPercent = 95
			}
			statusIcon = "📊" // Stabilizing
		} else if edgeCount > 0 {
			stabilityStatus = "learning"
			// Progress from 20% to 60% based on edge discovery
			edgeProgress := float64(edgeCount) / float64(g.totalObservations)
			if edgeProgress > 1.0 {
				edgeProgress = 1.0
			}
			stabilityPercent = 20 + int(edgeProgress*40)
			if stabilityPercent > 60 {
				stabilityPercent = 60
			}
			statusIcon = "🔄" // Learning
		}
	}

	// Calculate edge coverage (how many unique sources are part of at least one edge)
	var coveredSources = make(map[observer.SeriesID]bool)
	for edge := range g.knownEdges {
		coveredSources[edge.A] = true
		coveredSources[edge.B] = true
	}
	edgeCoverage := 0.0
	if len(g.uniqueSources) > 0 {
		edgeCoverage = float64(len(coveredSources)) / float64(len(g.uniqueSources)) * 100.0
	}

	return map[string]interface{}{
		"total_edges_seen":    len(g.knownEdges),
		"active_clusters":     len(g.clusters),
		"anomaly_buffer_size": len(g.anomalyBuffer),
		"current_time_bin":    g.currentTimeBin,
		"total_observations":  g.totalObservations,
		"last_new_edge_time":  g.lastNewEdgeTime,
		"time_since_new_edge": timeSinceNewEdge,
		"time_since_process":  timeSinceLastProcess,
		"stability_status":    stabilityStatus,
		"stability_percent":   stabilityPercent,
		"status_icon":         statusIcon,
		"frozen":              g.frozen,
		"unique_sources":      len(g.uniqueSources),
		"edge_coverage":       edgeCoverage,
		"available":           true, // Indicate that GraphSketch correlator is active
	}
}

// PrintDebugState prints debug information to stdout.
func (g *GraphSketchCorrelator) PrintDebugState() {
	g.mu.RLock()
	defer g.mu.RUnlock()

	fmt.Println("\n=== GraphSketch Correlator State ===")
	fmt.Printf("Edges learned: %d\n", len(g.knownEdges))
	fmt.Printf("Active clusters: %d\n", len(g.clusters))
	fmt.Printf("Anomaly buffer: %d\n", len(g.anomalyBuffer))
	fmt.Printf("Total anomaly pairs processed: %d\n", g.totalObservations)
	fmt.Printf("Unique sources observed: %d\n", len(g.uniqueSources))
	fmt.Printf("Frozen: %t\n", g.frozen)

	fmt.Println("\nTop co-occurrence edges (source pairs that anomaly together):")
	// Use locked version since we already hold the lock
	edges := g.getLearnedEdgesLocked()
	if len(edges) > 10 {
		edges = edges[:10]
	}
	for i, edge := range edges {
		fmt.Printf("  %d. %s ↔ %s (obs: %d, freq: %.2f)\n", i+1, edge.Source1, edge.Source2, edge.Observations, edge.Frequency)
	}
	fmt.Println("=====================================")
}

// GetExtraData implements CorrelatorDataProvider.
func (g *GraphSketchCorrelator) GetExtraData() interface{} {
	edges := g.GetLearnedEdges()
	if edges == nil {
		edges = []EdgeInfo{}
	}
	return edges
}

// Ensure GraphSketchCorrelator implements both interfaces
var _ observer.AnomalyProcessor = (*GraphSketchCorrelator)(nil)
var _ observer.CorrelationState = (*GraphSketchCorrelator)(nil)
