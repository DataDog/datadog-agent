// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"embed"
	"fmt"
	"io"
	"math"
	"sort"

	"github.com/DataDog/datadog-agent/comp/core/status"
)

//go:embed status_templates
var observerTemplatesFS embed.FS

// observerStatus provides observer data to agent status.
// Follows the same pattern as demultiplexerStatus.
type observerStatus struct {
	obs *observerImpl
}

func (s observerStatus) Name() string {
	return "Observer"
}

func (s observerStatus) Section() string {
	return "observer"
}

func (s observerStatus) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})
	s.populateStatus(stats)
	return stats
}

func (s observerStatus) populateStatus(stats map[string]interface{}) {
	if s.obs == nil {
		stats["observerStats"] = map[string]interface{}{
			"Score":          100,
			"Status":         "healthy",
			"Healthy":        true,
			"TotalAnomalies": 0,
			"UniqueMetrics":  0,
		}
		return
	}

	// Read raw anomalies from the observer
	rawAnomalies := s.obs.RawAnomalies()
	totalCount := s.obs.TotalAnomalyCount()
	uniqueSources := s.obs.UniqueAnomalySourceCount()

	// Compute a simple health score from anomaly count
	// (mirrors HealthCalculator logic but without requiring the full calculator)
	score := 100 - totalCount*2
	if score < 0 {
		score = 0
	}
	statusStr := "healthy"
	if score < 80 {
		statusStr = "degraded"
	}
	if score < 60 {
		statusStr = "warning"
	}
	if score < 30 {
		statusStr = "critical"
	}

	// Top anomalies by severity (deduped by source)
	type topAnomaly struct {
		Source    string
		Severity float64
		Direction string
	}

	bestBySource := make(map[string]topAnomaly)
	for _, a := range rawAnomalies {
		sev := 0.0
		if a.DebugInfo != nil {
			sev = a.DebugInfo.DeviationSigma
		}
		existing, ok := bestBySource[a.Source]
		if !ok || math.Abs(sev) > math.Abs(existing.Severity) {
			dir := "above"
			if sev < 0 {
				dir = "below"
			}
			bestBySource[a.Source] = topAnomaly{
				Source:    a.Source,
				Severity: math.Abs(sev),
				Direction: dir,
			}
		}
	}
	ranked := make([]topAnomaly, 0, len(bestBySource))
	for _, a := range bestBySource {
		ranked = append(ranked, a)
	}
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].Severity > ranked[j].Severity
	})
	if len(ranked) > 5 {
		ranked = ranked[:5]
	}

	// Collect findings from anomaly processors that implement FindingsProvider
	var findings []map[string]interface{}
	for _, proc := range s.obs.anomalyProcessors {
		if fp, ok := proc.(FindingsProvider); ok {
			for _, f := range fp.Findings() {
				if f.Confidence >= 0.5 && len(findings) < 3 {
					findings = append(findings, map[string]interface{}{
						"Summary": f.Summary,
					})
				}
			}
		}
	}

	// Health factors
	var factors []map[string]interface{}
	if totalCount > 0 {
		factors = append(factors, map[string]interface{}{
			"Name":         "anomaly_count",
			"Value":        float64(totalCount),
			"Contribution": totalCount * 2,
		})
	}

	stats["observerStats"] = map[string]interface{}{
		"Score":          score,
		"Status":         statusStr,
		"Healthy":        score >= 80,
		"Factors":        factors,
		"TotalAnomalies": totalCount,
		"UniqueMetrics":  uniqueSources,
		"TopAnomalies":   ranked,
		"Findings":       findings,
		"HasIncident":    score < 30,
	}
}

func (s observerStatus) JSON(_ bool, stats map[string]interface{}) error {
	s.populateStatus(stats)
	return nil
}

func (s observerStatus) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(observerTemplatesFS, "observer.tmpl", buffer, s.getStatusInfo())
}

func (s observerStatus) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(observerTemplatesFS, "observerHTML.tmpl", buffer, s.getStatusInfo())
}

// formatSigma formats a sigma value for display.
func formatSigma(v float64) string {
	return fmt.Sprintf("%.1fσ", v)
}
