// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"sync"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// HealthScore represents the current health state.
type HealthScore struct {
	Score       int            `json:"score"`       // 0-100, 100 = healthy
	LastUpdated int64          `json:"lastUpdated"` // unix timestamp
	Factors     []HealthFactor `json:"factors"`     // contributing factors
}

// HealthFactor describes a factor contributing to the health score.
type HealthFactor struct {
	Name        string  `json:"name"`
	Value       float64 `json:"value"`
	Weight      float64 `json:"weight"`
	Contribution int    `json:"contribution"` // points deducted
}

// HealthCalculator computes a health score based on anomalies and correlations.
type HealthCalculator struct {
	mu sync.RWMutex

	// Current state
	score       int
	lastUpdated int64
	factors     []HealthFactor

	// Configuration
	config HealthCalculatorConfig

	// History for tracking health over time
	history []HealthSnapshot
}

// HealthSnapshot records health at a point in time.
type HealthSnapshot struct {
	Timestamp int64 `json:"timestamp"`
	Score     int   `json:"score"`
}

// HealthCalculatorConfig configures the health calculator.
type HealthCalculatorConfig struct {
	// Weight multipliers for different factors
	// Tuned so metric anomalies drive the score:
	//   ~10 anomalies → health ~80 (degraded)
	//   ~20 anomalies → health ~60 (warning)
	//   ~50 anomalies → health ~0  (critical)
	AnomalyCountWeight  float64 // points per anomaly (default: 2)
	MaxSeverityWeight   float64 // points per severity unit, capped (default: 5)
	ClusterSizeWeight   float64 // points per signal in cluster (default: 1)
	ErrorLogRateWeight  float64 // supplemental, logs not yet processed effectively (default: 0.5)

	// Thresholds
	HealthyThreshold   int // score above this is healthy (default: 80)
	WarningThreshold   int // score below this is warning (default: 60)
	CriticalThreshold  int // score below this is critical (default: 30)

	// History
	MaxHistorySize int // max snapshots to keep (default: 100)
}

// DefaultHealthCalculatorConfig returns sensible defaults.
func DefaultHealthCalculatorConfig() HealthCalculatorConfig {
	return HealthCalculatorConfig{
		AnomalyCountWeight:  2,
		MaxSeverityWeight:   5,
		ClusterSizeWeight:   1,
		ErrorLogRateWeight:  0.5,
		HealthyThreshold:    80,
		WarningThreshold:    60,
		CriticalThreshold:   30,
		MaxHistorySize:      100,
	}
}

// NewHealthCalculator creates a new health calculator.
func NewHealthCalculator(cfg HealthCalculatorConfig) *HealthCalculator {
	if cfg.AnomalyCountWeight == 0 {
		cfg.AnomalyCountWeight = 2
	}
	if cfg.MaxSeverityWeight == 0 {
		cfg.MaxSeverityWeight = 5
	}
	if cfg.ClusterSizeWeight == 0 {
		cfg.ClusterSizeWeight = 1
	}
	if cfg.MaxHistorySize == 0 {
		cfg.MaxHistorySize = 100
	}

	return &HealthCalculator{
		score:   100,
		config:  cfg,
		history: make([]HealthSnapshot, 0, cfg.MaxHistorySize),
	}
}

// Update recalculates health based on current anomalies and correlations.
func (h *HealthCalculator) Update(
	anomalies []observerdef.AnomalyOutput,
	correlations []observerdef.ActiveCorrelation,
	errorLogCount int,
	timestamp int64,
) {
	h.mu.Lock()
	defer h.mu.Unlock()

	factors := make([]HealthFactor, 0, 4)
	totalDeduction := 0

	// Factor 1: Anomaly count
	anomalyCount := len(anomalies)
	if anomalyCount > 0 {
		deduction := int(float64(anomalyCount) * h.config.AnomalyCountWeight)
		factors = append(factors, HealthFactor{
			Name:        "anomaly_count",
			Value:       float64(anomalyCount),
			Weight:      h.config.AnomalyCountWeight,
			Contribution: deduction,
		})
		totalDeduction += deduction
	}

	// Factor 2: Max severity (based on z-score deviation if available)
	maxSeverity := 0.0
	for _, a := range anomalies {
		if a.DebugInfo != nil && a.DebugInfo.DeviationSigma > maxSeverity {
			maxSeverity = a.DebugInfo.DeviationSigma
		}
	}
	if maxSeverity > 0 {
		// Normalize severity: 3-5 sigma = mild, 5-10 = moderate, >10 = severe
		normalizedSeverity := math.Min(maxSeverity/5.0, 3.0) // cap at 3
		deduction := int(normalizedSeverity * h.config.MaxSeverityWeight)
		factors = append(factors, HealthFactor{
			Name:        "max_severity",
			Value:       maxSeverity,
			Weight:      h.config.MaxSeverityWeight,
			Contribution: deduction,
		})
		totalDeduction += deduction
	}

	// Factor 3: Correlation cluster size
	maxClusterSize := 0
	for _, c := range correlations {
		if len(c.SourceNames) > maxClusterSize {
			maxClusterSize = len(c.SourceNames)
		}
	}
	if maxClusterSize > 1 {
		deduction := int(float64(maxClusterSize) * h.config.ClusterSizeWeight)
		factors = append(factors, HealthFactor{
			Name:        "cluster_size",
			Value:       float64(maxClusterSize),
			Weight:      h.config.ClusterSizeWeight,
			Contribution: deduction,
		})
		totalDeduction += deduction
	}

	// Factor 4: Error log burst
	if errorLogCount > 10 {
		// Penalize bursts of error logs
		deduction := int(float64(errorLogCount-10) * h.config.ErrorLogRateWeight / 10)
		factors = append(factors, HealthFactor{
			Name:        "error_log_burst",
			Value:       float64(errorLogCount),
			Weight:      h.config.ErrorLogRateWeight,
			Contribution: deduction,
		})
		totalDeduction += deduction
	}

	// Calculate final score
	newScore := 100 - totalDeduction
	if newScore < 0 {
		newScore = 0
	}
	if newScore > 100 {
		newScore = 100
	}

	h.score = newScore
	h.lastUpdated = timestamp
	h.factors = factors

	// Record in history
	h.addToHistory(timestamp, newScore)
}

// addToHistory adds a snapshot, maintaining max size.
func (h *HealthCalculator) addToHistory(timestamp int64, score int) {
	snapshot := HealthSnapshot{
		Timestamp: timestamp,
		Score:     score,
	}

	if len(h.history) >= h.config.MaxHistorySize {
		// Shift left, drop oldest
		copy(h.history, h.history[1:])
		h.history = h.history[:len(h.history)-1]
	}
	h.history = append(h.history, snapshot)
}

// GetScore returns the current health score.
func (h *HealthCalculator) GetScore() HealthScore {
	h.mu.RLock()
	defer h.mu.RUnlock()

	factorsCopy := make([]HealthFactor, len(h.factors))
	copy(factorsCopy, h.factors)

	return HealthScore{
		Score:       h.score,
		LastUpdated: h.lastUpdated,
		Factors:     factorsCopy,
	}
}

// GetHistory returns health score history.
func (h *HealthCalculator) GetHistory() []HealthSnapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]HealthSnapshot, len(h.history))
	copy(result, h.history)
	return result
}

// GetStatus returns a human-readable status based on score.
func (h *HealthCalculator) GetStatus() string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.score >= h.config.HealthyThreshold {
		return "healthy"
	}
	if h.score >= h.config.WarningThreshold {
		return "warning"
	}
	if h.score >= h.config.CriticalThreshold {
		return "degraded"
	}
	return "critical"
}

// Reset resets the health calculator to initial state.
func (h *HealthCalculator) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.score = 100
	h.lastUpdated = 0
	h.factors = nil
	h.history = h.history[:0]
}

// HealthResponse is the API response format for health endpoint.
type HealthResponse struct {
	Score       int              `json:"score"`
	Status      string           `json:"status"` // healthy, warning, degraded, critical
	LastUpdated int64            `json:"lastUpdated"`
	Factors     []HealthFactor   `json:"factors"`
	History     []HealthSnapshot `json:"history,omitempty"`
}
