// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// ContextPacket is a snapshot of system state captured when health drops.
type ContextPacket struct {
	ID        string `json:"id"`
	Timestamp int64  `json:"timestamp"` // unix seconds

	// Health state
	HealthBefore int `json:"healthBefore"`
	HealthAfter  int `json:"healthAfter"`
	HealthDrop   int `json:"healthDrop"` // positive = dropped

	// Trigger
	TriggerReason string                    `json:"triggerReason"`
	TriggerAnomaly *observerdef.AnomalyOutput `json:"triggerAnomaly,omitempty"`

	// Correlations
	Correlations []CorrelationSnapshot `json:"correlations,omitempty"`

	// Logs (deduped patterns + error samples)
	LogPatterns []LogPatternSummary `json:"logPatterns,omitempty"`
	ErrorLogs   []BufferedLog       `json:"errorLogs,omitempty"`

	// Metrics (series with anomalies)
	AffectedSeries []SeriesSnapshot `json:"affectedSeries,omitempty"`

	// All anomalies in the packet window
	Anomalies []AnomalySnapshot `json:"anomalies,omitempty"`
}

// CorrelationSnapshot is a simplified view of a correlation for the packet.
type CorrelationSnapshot struct {
	Pattern     string   `json:"pattern"`
	Title       string   `json:"title"`
	Sources     []string `json:"sources"`
	AnomalyCount int     `json:"anomalyCount"`
	FirstSeen   int64    `json:"firstSeen"`
	LastUpdated int64    `json:"lastUpdated"`
}

// SeriesSnapshot captures key points from a time series.
type SeriesSnapshot struct {
	Name       string    `json:"name"`
	Tags       []string  `json:"tags,omitempty"`
	Min        float64   `json:"min"`
	Max        float64   `json:"max"`
	Mean       float64   `json:"mean"`
	LastValue  float64   `json:"lastValue"`
	DataPoints int       `json:"dataPoints"`
}

// AnomalySnapshot is a simplified anomaly view for the packet.
type AnomalySnapshot struct {
	Source      string  `json:"source"`
	Analyzer    string  `json:"analyzer"`
	Title       string  `json:"title"`
	Timestamp   int64   `json:"timestamp"`
	Severity    float64 `json:"severity,omitempty"` // sigma deviation
}

// ContextPacketGenerator generates context packets on health drops.
type ContextPacketGenerator struct {
	mu sync.Mutex

	config ContextPacketConfig

	// State
	lastHealthScore int
	packets         []ContextPacket
	lastPacketTime  int64 // rate limiting
}

// ContextPacketConfig configures the context packet generator.
type ContextPacketConfig struct {
	// Trigger thresholds
	HealthDropThreshold int   // minimum drop to trigger (default: 20)
	MinHealthScore      int   // also trigger if health drops below this (default: 40)
	MinPacketInterval   int64 // minimum seconds between packets (default: 60)

	// Content limits
	MaxLogPatterns int // max patterns to include (default: 50)
	MaxErrorLogs   int // max error logs to include (default: 100)
	MaxAnomalies   int // max anomalies to include (default: 50)

	// Storage
	OutputDir        string // directory for packet files (empty = don't write)
	MaxStoredPackets int    // max packets to keep in memory (default: 10)
}

// DefaultContextPacketConfig returns sensible defaults.
func DefaultContextPacketConfig() ContextPacketConfig {
	return ContextPacketConfig{
		HealthDropThreshold: 20,
		MinHealthScore:      40,
		MinPacketInterval:   60,
		MaxLogPatterns:      50,
		MaxErrorLogs:        100,
		MaxAnomalies:        50,
		MaxStoredPackets:    10,
	}
}

// NewContextPacketGenerator creates a new context packet generator.
func NewContextPacketGenerator(cfg ContextPacketConfig) *ContextPacketGenerator {
	if cfg.HealthDropThreshold == 0 {
		cfg.HealthDropThreshold = 20
	}
	if cfg.MinHealthScore == 0 {
		cfg.MinHealthScore = 40
	}
	if cfg.MinPacketInterval == 0 {
		cfg.MinPacketInterval = 60
	}
	if cfg.MaxLogPatterns == 0 {
		cfg.MaxLogPatterns = 50
	}
	if cfg.MaxErrorLogs == 0 {
		cfg.MaxErrorLogs = 100
	}
	if cfg.MaxAnomalies == 0 {
		cfg.MaxAnomalies = 50
	}
	if cfg.MaxStoredPackets == 0 {
		cfg.MaxStoredPackets = 10
	}

	return &ContextPacketGenerator{
		config:          cfg,
		lastHealthScore: 100,
		packets:         make([]ContextPacket, 0, cfg.MaxStoredPackets),
	}
}

// CheckAndGenerate checks if a context packet should be generated based on health change.
// Returns the generated packet if one was created, nil otherwise.
func (g *ContextPacketGenerator) CheckAndGenerate(
	newHealthScore int,
	timestamp int64,
	anomalies []observerdef.AnomalyOutput,
	correlations []observerdef.ActiveCorrelation,
	logPatterns []LogPatternSummary,
	errorLogs []BufferedLog,
) *ContextPacket {
	g.mu.Lock()
	defer g.mu.Unlock()

	oldScore := g.lastHealthScore
	g.lastHealthScore = newHealthScore

	// Check if we should generate a packet
	healthDrop := oldScore - newHealthScore
	shouldGenerate := false
	triggerReason := ""

	if healthDrop >= g.config.HealthDropThreshold {
		shouldGenerate = true
		triggerReason = fmt.Sprintf("health dropped %d points (from %d to %d)", healthDrop, oldScore, newHealthScore)
	} else if newHealthScore < g.config.MinHealthScore && oldScore >= g.config.MinHealthScore {
		shouldGenerate = true
		triggerReason = fmt.Sprintf("health crossed critical threshold (now %d)", newHealthScore)
	}

	if !shouldGenerate {
		return nil
	}

	// Rate limiting
	if timestamp-g.lastPacketTime < g.config.MinPacketInterval {
		return nil
	}

	// Generate packet
	packet := g.generatePacket(timestamp, oldScore, newHealthScore, triggerReason, anomalies, correlations, logPatterns, errorLogs)

	// Store packet
	g.lastPacketTime = timestamp
	g.addPacket(packet)

	// Write to disk if configured
	if g.config.OutputDir != "" {
		g.writePacketToFile(packet)
	}

	return &packet
}

// generatePacket creates a new context packet.
func (g *ContextPacketGenerator) generatePacket(
	timestamp int64,
	healthBefore, healthAfter int,
	triggerReason string,
	anomalies []observerdef.AnomalyOutput,
	correlations []observerdef.ActiveCorrelation,
	logPatterns []LogPatternSummary,
	errorLogs []BufferedLog,
) ContextPacket {
	packet := ContextPacket{
		ID:            fmt.Sprintf("ctx_%d", timestamp),
		Timestamp:     timestamp,
		HealthBefore:  healthBefore,
		HealthAfter:   healthAfter,
		HealthDrop:    healthBefore - healthAfter,
		TriggerReason: triggerReason,
	}

	// Find the most recent/severe anomaly as trigger
	if len(anomalies) > 0 {
		var triggerAnomaly *observerdef.AnomalyOutput
		maxSeverity := 0.0
		for i := range anomalies {
			a := &anomalies[i]
			severity := 0.0
			if a.DebugInfo != nil {
				severity = a.DebugInfo.DeviationSigma
			}
			if triggerAnomaly == nil || severity > maxSeverity {
				triggerAnomaly = a
				maxSeverity = severity
			}
		}
		packet.TriggerAnomaly = triggerAnomaly
	}

	// Add correlations
	for _, c := range correlations {
		packet.Correlations = append(packet.Correlations, CorrelationSnapshot{
			Pattern:      c.Pattern,
			Title:        c.Title,
			Sources:      c.SourceNames,
			AnomalyCount: len(c.Anomalies),
			FirstSeen:    c.FirstSeen,
			LastUpdated:  c.LastUpdated,
		})
	}

	// Add log patterns (limited)
	if len(logPatterns) > g.config.MaxLogPatterns {
		logPatterns = logPatterns[:g.config.MaxLogPatterns]
	}
	packet.LogPatterns = logPatterns

	// Add error logs (limited)
	if len(errorLogs) > g.config.MaxErrorLogs {
		errorLogs = errorLogs[len(errorLogs)-g.config.MaxErrorLogs:]
	}
	packet.ErrorLogs = errorLogs

	// Add anomalies (limited)
	for i, a := range anomalies {
		if i >= g.config.MaxAnomalies {
			break
		}
		severity := 0.0
		if a.DebugInfo != nil {
			severity = a.DebugInfo.DeviationSigma
		}
		packet.Anomalies = append(packet.Anomalies, AnomalySnapshot{
			Source:    a.Source,
			Analyzer:  a.AnalyzerName,
			Title:     a.Title,
			Timestamp: a.Timestamp,
			Severity:  severity,
		})
	}

	return packet
}

// addPacket adds a packet to storage, maintaining max size.
func (g *ContextPacketGenerator) addPacket(packet ContextPacket) {
	if len(g.packets) >= g.config.MaxStoredPackets {
		// Remove oldest
		copy(g.packets, g.packets[1:])
		g.packets = g.packets[:len(g.packets)-1]
	}
	g.packets = append(g.packets, packet)
}

// writePacketToFile writes the packet to a JSON file.
func (g *ContextPacketGenerator) writePacketToFile(packet ContextPacket) error {
	if g.config.OutputDir == "" {
		return nil
	}

	// Ensure directory exists
	if err := os.MkdirAll(g.config.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create filename with timestamp
	t := time.Unix(packet.Timestamp, 0).UTC()
	filename := fmt.Sprintf("context_%s.json", t.Format("2006-01-02T15-04-05"))
	filepath := filepath.Join(g.config.OutputDir, filename)

	// Write JSON
	data, err := json.MarshalIndent(packet, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal packet: %w", err)
	}

	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return fmt.Errorf("failed to write packet file: %w", err)
	}

	fmt.Printf("  Context packet written: %s\n", filename)
	return nil
}

// GetPackets returns all stored packets.
func (g *ContextPacketGenerator) GetPackets() []ContextPacket {
	g.mu.Lock()
	defer g.mu.Unlock()

	result := make([]ContextPacket, len(g.packets))
	copy(result, g.packets)
	return result
}

// GetLatestPacket returns the most recent packet, or nil if none.
func (g *ContextPacketGenerator) GetLatestPacket() *ContextPacket {
	g.mu.Lock()
	defer g.mu.Unlock()

	if len(g.packets) == 0 {
		return nil
	}
	packet := g.packets[len(g.packets)-1]
	return &packet
}

// Reset clears all stored packets and resets state.
func (g *ContextPacketGenerator) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.lastHealthScore = 100
	g.packets = g.packets[:0]
	g.lastPacketTime = 0
}

// SetHealthScore updates the last known health score without generating a packet.
// Use this when initializing or after loading a new scenario.
func (g *ContextPacketGenerator) SetHealthScore(score int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.lastHealthScore = score
}
