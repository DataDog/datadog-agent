// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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

// Finding is a generic correlator output. All correlators translate their
// internal state into Findings so the flare/report generator doesn't need
// to know about specific correlator types.
type Finding struct {
	// What kind of insight this is (user-facing, not algorithm name)
	Category string `json:"category"` // "simultaneous", "causal", "co_occurrence", "learned"

	// Human-readable one-liner summarizing the finding
	Summary string `json:"summary"` // e.g. "io.w_s leads container.memory by 8s (100% confidence)"

	// Metrics involved
	Sources []string `json:"sources"`

	// Strength of the signal (0-1), comparable across correlator types
	Confidence float64 `json:"confidence"`

	// When this was observed
	Timestamp int64 `json:"timestamp,omitempty"`
	EndTime   int64 `json:"endTime,omitempty"`
}

// FindingsProvider produces Findings from correlator state.
// Each correlator implements this to contribute to flares/reports.
type FindingsProvider interface {
	Findings() []Finding
}

// FlareDataProvider is an interface for components that can provide flare data.
type FlareDataProvider interface {
	GetHealth() HealthResponse
	GetContextPackets() []ContextPacket
	GetLogBufferStats() LogBufferStats
	GetAnomalies() []AnomalySnapshot
	GetCorrelations() []CorrelationSnapshot
	GetFindings() []Finding
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

	// Add findings from all enabled correlators
	findings := provider.GetFindings()
	if len(findings) > 0 {
		if data, err := json.MarshalIndent(findings, "", "  "); err == nil {
			fb.AddFile("observer/findings.json", data) //nolint:errcheck
		}
	}

	// Add a human-readable incident report
	summaryText := generateIncidentReport(health, anomalies, contextPackets, findings)
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

// formatTime formats a unix timestamp as HH:MM:SS UTC.
func formatTime(ts int64) string {
	if ts == 0 {
		return "unknown"
	}
	return time.Unix(ts, 0).UTC().Format("15:04:05")
}

// generateIncidentReport creates a human-readable incident report for flares.
// Focuses on timeline, causality, and actionable information — not raw data dumps.
func generateIncidentReport(
	health HealthResponse,
	anomalies []AnomalySnapshot,
	packets []ContextPacket,
	findings []Finding,
) string {
	var b strings.Builder

	// --- Header ---
	b.WriteString("=== Observer Incident Report ===\n\n")
	b.WriteString(fmt.Sprintf("Health: %d/100 (%s) at %s UTC\n",
		health.Score, strings.ToUpper(health.Status), formatTime(health.LastUpdated)))

	// Unique sources
	sources := make(map[string]bool)
	for _, a := range anomalies {
		sources[a.Source] = true
	}
	b.WriteString(fmt.Sprintf("  %d anomalies across %d metrics\n", len(anomalies), len(sources)))

	if len(health.Factors) > 0 {
		b.WriteString("  Factors:\n")
		for _, f := range health.Factors {
			b.WriteString(fmt.Sprintf("    -%d pts  %s (%.1f)\n", f.Contribution, f.Name, f.Value))
		}
	}
	b.WriteString("\n")

	// --- Timeline ---
	b.WriteString("--- Timeline ---\n")

	// Build timeline events from anomalies, sorted by timestamp
	type timelineEvent struct {
		timestamp int64
		text      string
	}
	var events []timelineEvent

	// Group anomalies by timestamp to collapse simultaneous ones
	byTimestamp := make(map[int64][]AnomalySnapshot)
	for _, a := range anomalies {
		byTimestamp[a.Timestamp] = append(byTimestamp[a.Timestamp], a)
	}

	// Sort timestamps
	timestamps := make([]int64, 0, len(byTimestamp))
	for ts := range byTimestamp {
		timestamps = append(timestamps, ts)
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })

	for _, ts := range timestamps {
		group := byTimestamp[ts]
		if len(group) == 1 {
			a := group[0]
			events = append(events, timelineEvent{
				timestamp: ts,
				text: fmt.Sprintf("SHIFT     %s  %.1fσ [%s]",
					a.Source, math.Abs(a.Severity), a.Analyzer),
			})
		} else {
			// Multiple anomalies at same timestamp — summarize
			sourceList := make([]string, 0, len(group))
			maxSev := 0.0
			for _, a := range group {
				sourceList = append(sourceList, a.Source)
				if math.Abs(a.Severity) > maxSev {
					maxSev = math.Abs(a.Severity)
				}
			}
			// Collapse to unique source prefixes
			events = append(events, timelineEvent{
				timestamp: ts,
				text: fmt.Sprintf("CLUSTER   %d metrics shifted simultaneously (max %.1fσ): %s",
					len(group), maxSev, strings.Join(sourceList, ", ")),
			})
		}
	}

	// Add context packet triggers
	for _, p := range packets {
		events = append(events, timelineEvent{
			timestamp: p.Timestamp,
			text:      fmt.Sprintf("TRIGGER   Health dropped %d → %d", p.HealthBefore, p.HealthAfter),
		})
	}

	// Sort all events
	sort.Slice(events, func(i, j int) bool { return events[i].timestamp < events[j].timestamp })

	for _, e := range events {
		b.WriteString(fmt.Sprintf("%s  %s\n", formatTime(e.timestamp), e.text))
	}
	b.WriteString("\n")

	// --- Findings (from all enabled correlators) ---
	if len(findings) > 0 {
		// Group findings by category for readability
		byCategory := make(map[string][]Finding)
		for _, f := range findings {
			byCategory[f.Category] = append(byCategory[f.Category], f)
		}

		categoryLabels := map[string]string{
			"simultaneous":  "Metrics that shifted at the same time",
			"causal":        "Causal chains (A leads B)",
			"co_occurrence": "Unexpected co-occurrences",
			"learned":       "Frequently co-occurring patterns",
		}
		// Render in a stable order
		for _, cat := range []string{"simultaneous", "causal", "co_occurrence", "learned"} {
			group := byCategory[cat]
			if len(group) == 0 {
				continue
			}
			label := categoryLabels[cat]
			if label == "" {
				label = cat
			}

			b.WriteString(fmt.Sprintf("--- %s (%d) ---\n", label, len(group)))

			// Sort by confidence descending
			sort.Slice(group, func(i, j int) bool {
				return group[i].Confidence > group[j].Confidence
			})

			for i, f := range group {
				if i >= 10 {
					b.WriteString(fmt.Sprintf("  ... and %d more\n", len(group)-10))
					break
				}
				ts := ""
				if f.Timestamp > 0 {
					ts = fmt.Sprintf(" @ %s", formatTime(f.Timestamp))
				}
				b.WriteString(fmt.Sprintf("  %s%s\n", f.Summary, ts))
			}
			b.WriteString("\n")
		}
	}

	// --- Context Packets ---
	for _, p := range packets {
		b.WriteString(fmt.Sprintf("--- Incident Snapshot: %s ---\n", p.ID))
		b.WriteString(fmt.Sprintf("Time: %s UTC | Health: %d → %d\n",
			formatTime(p.Timestamp), p.HealthBefore, p.HealthAfter))
		b.WriteString(fmt.Sprintf("Trigger: %s\n", p.TriggerReason))

		// Anomalous logs only (error/warn)
		errorLogCount := len(p.ErrorLogs)
		if errorLogCount > 0 {
			b.WriteString(fmt.Sprintf("\nAnomalous Logs (%d):\n", errorLogCount))
			shown := 0
			for _, l := range p.ErrorLogs {
				if shown >= 10 {
					b.WriteString(fmt.Sprintf("  ... and %d more\n", errorLogCount-10))
					break
				}
				content := l.Content
				if len(content) > 120 {
					content = content[:120] + "..."
				}
				b.WriteString(fmt.Sprintf("  [%s] %s\n", l.Status, content))
				shown++
			}
		} else {
			b.WriteString("\nAnomalous Logs: none\n")
		}
		b.WriteString("\n")
	}

	// --- Top Anomalies by Severity ---
	b.WriteString("--- Top Anomalies by Severity ---\n")

	// Dedup by source, keep highest severity
	bestBySource := make(map[string]AnomalySnapshot)
	for _, a := range anomalies {
		existing, ok := bestBySource[a.Source]
		if !ok || math.Abs(a.Severity) > math.Abs(existing.Severity) {
			bestBySource[a.Source] = a
		}
	}

	ranked := make([]AnomalySnapshot, 0, len(bestBySource))
	for _, a := range bestBySource {
		ranked = append(ranked, a)
	}
	sort.Slice(ranked, func(i, j int) bool {
		return math.Abs(ranked[i].Severity) > math.Abs(ranked[j].Severity)
	})

	for i, a := range ranked {
		if i >= 15 {
			b.WriteString(fmt.Sprintf("  ... and %d more\n", len(ranked)-15))
			break
		}
		direction := "above"
		if a.Severity < 0 {
			direction = "below"
		}
		b.WriteString(fmt.Sprintf("%2d. %s  %.1fσ %s baseline [%s]\n",
			i+1, a.Source, math.Abs(a.Severity), direction, a.Analyzer))
	}

	return b.String()
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

	// Write human-readable incident report
	// Note: WriteFlareToDirectory doesn't have access to correlator findings.
	// Use fillObserverFlare (via FlareBuilder) for the full report.
	summaryText := generateIncidentReport(health, anomalies, contextPackets, nil)
	if err := os.WriteFile(filepath.Join(dir, "summary.txt"), []byte(summaryText), 0644); err != nil {
		return err
	}

	return nil
}
