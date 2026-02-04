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

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
)

// ObserverFlareData contains all data that the observer contributes to a flare.
type ObserverFlareData struct {
	// Health information
	Health HealthResponse `json:"health"`

	// Anomaly summary
	AnomalySummary AnomalySummary `json:"anomalySummary"`

	// Context packets (recent incident snapshots)
	ContextPackets []ContextPacket `json:"contextPackets"`

	// Log buffer statistics
	LogBufferStats LogBufferStats `json:"logBufferStats"`
}

// AnomalySummary provides aggregate statistics about detected anomalies.
type AnomalySummary struct {
	TotalAnomalies   int                       `json:"totalAnomalies"`
	ByAnalyzer       map[string]int            `json:"byAnalyzer"`
	BySource         map[string]int            `json:"bySource"`
	RecentAnomalies  []AnomalySnapshot         `json:"recentAnomalies,omitempty"`
	Correlations     []CorrelationSnapshot     `json:"correlations,omitempty"`
	TimeRange        *FlareTimeRange           `json:"timeRange,omitempty"`
}

// FlareTimeRange represents the time window covered by the data.
type FlareTimeRange struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

// FlareDataProvider is an interface for components that can provide flare data.
type FlareDataProvider interface {
	GetHealth() HealthResponse
	GetContextPackets() []ContextPacket
	GetLogBufferStats() LogBufferStats
	GetAnomalies() []AnomalySnapshot
	GetCorrelations() []CorrelationSnapshot
}

// NewFlareProvider creates a flare provider callback for the observer component.
// The dataProvider should implement FlareDataProvider to supply the actual data.
func NewFlareProvider(dataProvider FlareDataProvider) flaretypes.Provider {
	return flaretypes.NewProvider(func(fb flaretypes.FlareBuilder) error {
		return fillObserverFlare(fb, dataProvider)
	})
}

// fillObserverFlare adds observer data to a flare.
func fillObserverFlare(fb flaretypes.FlareBuilder, provider FlareDataProvider) error {
	if provider == nil {
		fb.AddFile("observer/observer.log", []byte("Observer data provider not available")) //nolint:errcheck
		return nil
	}

	// Collect all data
	health := provider.GetHealth()
	contextPackets := provider.GetContextPackets()
	logBufferStats := provider.GetLogBufferStats()
	anomalies := provider.GetAnomalies()
	correlations := provider.GetCorrelations()

	// Build anomaly summary
	summary := buildAnomalySummary(anomalies, correlations)

	// Create the combined flare data
	flareData := ObserverFlareData{
		Health:         health,
		AnomalySummary: summary,
		ContextPackets: contextPackets,
		LogBufferStats: logBufferStats,
	}

	// Add main observer data file
	if data, err := json.MarshalIndent(flareData, "", "  "); err == nil {
		fb.AddFile("observer/observer_data.json", data) //nolint:errcheck
	}

	// Add health score separately for easy access
	if data, err := json.MarshalIndent(health, "", "  "); err == nil {
		fb.AddFile("observer/health_score.json", data) //nolint:errcheck
	}

	// Add each context packet as a separate file
	for _, packet := range contextPackets {
		filename := fmt.Sprintf("observer/context_packets/%s.json", packet.ID)
		if data, err := json.MarshalIndent(packet, "", "  "); err == nil {
			fb.AddFile(filename, data) //nolint:errcheck
		}
	}

	// Add anomaly summary
	if data, err := json.MarshalIndent(summary, "", "  "); err == nil {
		fb.AddFile("observer/anomaly_summary.json", data) //nolint:errcheck
	}

	// Add log buffer stats
	if data, err := json.MarshalIndent(logBufferStats, "", "  "); err == nil {
		fb.AddFile("observer/log_buffer_stats.json", data) //nolint:errcheck
	}

	// Add a human-readable summary
	summaryText := generateHumanReadableSummary(health, summary, contextPackets)
	fb.AddFile("observer/summary.txt", []byte(summaryText)) //nolint:errcheck

	return nil
}

// buildAnomalySummary creates an anomaly summary from the raw data.
func buildAnomalySummary(anomalies []AnomalySnapshot, correlations []CorrelationSnapshot) AnomalySummary {
	summary := AnomalySummary{
		TotalAnomalies: len(anomalies),
		ByAnalyzer:     make(map[string]int),
		BySource:       make(map[string]int),
	}

	var minTime, maxTime int64
	for _, a := range anomalies {
		summary.ByAnalyzer[a.Analyzer]++
		summary.BySource[a.Source]++

		if minTime == 0 || a.Timestamp < minTime {
			minTime = a.Timestamp
		}
		if a.Timestamp > maxTime {
			maxTime = a.Timestamp
		}
	}

	// Add recent anomalies (last 20)
	recentCount := 20
	if len(anomalies) < recentCount {
		recentCount = len(anomalies)
	}
	if recentCount > 0 {
		summary.RecentAnomalies = anomalies[len(anomalies)-recentCount:]
	}

	// Add correlations
	summary.Correlations = correlations

	// Set time range if we have data
	if minTime > 0 {
		summary.TimeRange = &FlareTimeRange{
			Start: minTime,
			End:   maxTime,
		}
	}

	return summary
}

// generateHumanReadableSummary creates a text summary for quick reading.
func generateHumanReadableSummary(health HealthResponse, summary AnomalySummary, packets []ContextPacket) string {
	var result string

	result += "=== Observer Health Summary ===\n\n"
	result += fmt.Sprintf("Health Score: %d/100 (%s)\n", health.Score, health.Status)
	result += fmt.Sprintf("Last Updated: %d\n\n", health.LastUpdated)

	if len(health.Factors) > 0 {
		result += "Contributing Factors:\n"
		for _, f := range health.Factors {
			result += fmt.Sprintf("  - %s: %.2f (-%d points)\n", f.Name, f.Value, f.Contribution)
		}
		result += "\n"
	}

	result += "=== Anomaly Summary ===\n\n"
	result += fmt.Sprintf("Total Anomalies: %d\n", summary.TotalAnomalies)

	if len(summary.ByAnalyzer) > 0 {
		result += "\nBy Analyzer:\n"
		for analyzer, count := range summary.ByAnalyzer {
			result += fmt.Sprintf("  - %s: %d\n", analyzer, count)
		}
	}

	if len(summary.Correlations) > 0 {
		result += fmt.Sprintf("\nActive Correlations: %d\n", len(summary.Correlations))
		for _, c := range summary.Correlations {
			result += fmt.Sprintf("  - %s: %d sources\n", c.Pattern, len(c.Sources))
		}
	}

	result += "\n=== Context Packets ===\n\n"
	result += fmt.Sprintf("Total Packets: %d\n", len(packets))
	for _, p := range packets {
		result += fmt.Sprintf("  - %s: health %d → %d (drop: %d)\n",
			p.ID, p.HealthBefore, p.HealthAfter, p.HealthDrop)
	}

	return result
}

// WriteFlareToDirectory writes observer flare data to a directory.
// This is useful for the testbench to manually export flare-like data.
func WriteFlareToDirectory(dir string, provider FlareDataProvider) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create flare directory: %w", err)
	}

	// Collect data
	health := provider.GetHealth()
	contextPackets := provider.GetContextPackets()
	logBufferStats := provider.GetLogBufferStats()
	anomalies := provider.GetAnomalies()
	correlations := provider.GetCorrelations()
	summary := buildAnomalySummary(anomalies, correlations)

	// Write health score
	if data, err := json.MarshalIndent(health, "", "  "); err == nil {
		if err := os.WriteFile(filepath.Join(dir, "health_score.json"), data, 0644); err != nil {
			return err
		}
	}

	// Write anomaly summary
	if data, err := json.MarshalIndent(summary, "", "  "); err == nil {
		if err := os.WriteFile(filepath.Join(dir, "anomaly_summary.json"), data, 0644); err != nil {
			return err
		}
	}

	// Write log buffer stats
	if data, err := json.MarshalIndent(logBufferStats, "", "  "); err == nil {
		if err := os.WriteFile(filepath.Join(dir, "log_buffer_stats.json"), data, 0644); err != nil {
			return err
		}
	}

	// Write context packets
	if len(contextPackets) > 0 {
		packetsDir := filepath.Join(dir, "context_packets")
		if err := os.MkdirAll(packetsDir, 0755); err != nil {
			return err
		}
		for _, p := range contextPackets {
			if data, err := json.MarshalIndent(p, "", "  "); err == nil {
				filename := filepath.Join(packetsDir, fmt.Sprintf("%s.json", p.ID))
				if err := os.WriteFile(filename, data, 0644); err != nil {
					return err
				}
			}
		}
	}

	// Write human-readable summary
	summaryText := generateHumanReadableSummary(health, summary, contextPackets)
	if err := os.WriteFile(filepath.Join(dir, "summary.txt"), []byte(summaryText), 0644); err != nil {
		return err
	}

	return nil
}
