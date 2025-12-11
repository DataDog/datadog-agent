// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package summary

import (
	"strings"
	"time"

	mh "github.com/DataDog/datadog-agent/pkg/aggregator/metric_history"
)

// AnomalySummarySystem wraps ClusterSet and provides the integration interface
type AnomalySummarySystem struct {
	clusters *ClusterSet
}

// NewAnomalySummarySystem creates a new summary system with default config
func NewAnomalySummarySystem() *AnomalySummarySystem {
	return &AnomalySummarySystem{
		clusters: NewClusterSet(DefaultClusterConfig()),
	}
}

// NewAnomalySummarySystemWithConfig creates with custom config
func NewAnomalySummarySystemWithConfig(cfg ClusterConfig) *AnomalySummarySystem {
	return &AnomalySummarySystem{
		clusters: NewClusterSet(cfg),
	}
}

// ProcessAnomalies converts metric_history.Anomaly events to AnomalyEvent and adds them
// Returns cluster summaries that are ready to be logged
func (s *AnomalySummarySystem) ProcessAnomalies(anomalies []mh.Anomaly, now time.Time) []ClusterSummary {
	// TODO: implement
	// 1. Convert each mh.Anomaly to AnomalyEvent
	for _, a := range anomalies {
		event := convertAnomaly(a)
		s.clusters.Add(event)
	}

	// 2. Call Tick to update states
	s.clusters.Tick(now)

	// 3. Return summaries for active/stabilizing clusters
	var summaries []ClusterSummary
	for _, cluster := range s.clusters.Clusters() {
		if cluster.State == Active || cluster.State == Stabilizing {
			summary := Summarize(cluster)
			summaries = append(summaries, summary)
		}
	}

	return summaries
}

// convertAnomaly converts a metric_history.Anomaly to a summary.AnomalyEvent
func convertAnomaly(a mh.Anomaly) AnomalyEvent {
	// TODO: implement
	// Map fields: Timestamp, Metric (from SeriesKey.Name), Tags (from SeriesKey.Tags),
	// Severity, Direction (from Message), Magnitude (derive from message or use severity)

	// Convert timestamp
	timestamp := time.Unix(a.Timestamp, 0)

	// Parse tags from []string{"key:value"} to map[string]string
	tags := make(map[string]string)
	for _, tag := range a.SeriesKey.Tags {
		parts := strings.SplitN(tag, ":", 2)
		if len(parts) == 2 {
			tags[parts[0]] = parts[1]
		}
	}

	// Extract direction from message (look for "increase" or "decrease")
	direction := ""
	lowerMsg := strings.ToLower(a.Message)
	if strings.Contains(lowerMsg, "increase") {
		direction = "increase"
	} else if strings.Contains(lowerMsg, "decrease") {
		direction = "decrease"
	}

	// Use severity * 10 as a rough magnitude estimate
	magnitude := a.Severity * 10

	return AnomalyEvent{
		Timestamp: timestamp,
		Metric:    a.SeriesKey.Name,
		Tags:      tags,
		Severity:  a.Severity,
		Direction: direction,
		Magnitude: magnitude,
	}
}
