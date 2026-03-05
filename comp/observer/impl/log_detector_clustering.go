// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"strings"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// clusterState tracks the exemplar and count for a single cluster (token-count pattern).
type clusterState struct {
	Exemplar string
	Count    int64
}

// LogClusteringDetector is a simple example log detector that clusters logs
// by the number of whitespace-delimited tokens. It maintains a map of
// token-count → {exemplar, count} and emits a count metric for each pattern seen.
//
// This is intentionally trivial — the point is to demonstrate how a stateful
// LogDetector can maintain clustering state and emit metrics that feed into
// the rest of the observer pipeline (storage → MetricsDetector → Correlator).
type LogClusteringDetector struct {
	// Clusters maps token count → cluster state.
	Clusters map[int]*clusterState
}

// Name returns the detector name.
func (d *LogClusteringDetector) Name() string {
	return "log_clustering"
}

// Process examines a log, assigns it to a cluster, and emits a count metric.
// If the log represents a never-before-seen cluster, it also emits an anomaly.
//
// Note that classification and state update are combined in ingest() — this
// is realistic because many clustering algorithms (e.g. Drain) mutate their
// internal model as a side effect of classification.
func (d *LogClusteringDetector) Process(log observer.LogView) observer.LogDetectionResult {
	content := strings.TrimSpace(string(log.GetContent()))
	if content == "" {
		return observer.LogDetectionResult{}
	}

	pattern, isNew := d.ingest(content)

	result := observer.LogDetectionResult{
		Metrics: []observer.MetricOutput{{
			Name:  fmt.Sprintf("log.cluster.tokens_%d.count", pattern),
			Value: 1.0,
			Tags:  log.GetTags(),
		}},
	}

	if isNew {
		result.Anomalies = []observer.Anomaly{{
			Type:         observer.AnomalyTypeLog,
			Source:       "log.clustering",
			DetectorName: d.Name(),
			Title:        fmt.Sprintf("New log cluster: tokens_%d", pattern),
			Description:  fmt.Sprintf("First occurrence of a %d-token log pattern: %s", pattern, content),
			Tags:         log.GetTags(),
			Timestamp:    log.GetTimestampMs() / 1000,
		}}
	}

	return result
}

// ingest classifies a log line and updates cluster state in one step.
// This stub classifies by whitespace token count — a real implementation
// might use something like Drain, locality-sensitive hashing, or an LLM
// embedding. Returns the pattern ID and whether this is a new cluster.
func (d *LogClusteringDetector) ingest(content string) (pattern int, isNew bool) {
	pattern = len(strings.Fields(content))

	if d.Clusters == nil {
		d.Clusters = make(map[int]*clusterState)
	}
	cs, exists := d.Clusters[pattern]
	if !exists {
		cs = &clusterState{Exemplar: content}
		d.Clusters[pattern] = cs
		isNew = true
	}
	cs.Count++
	return pattern, isNew
}
