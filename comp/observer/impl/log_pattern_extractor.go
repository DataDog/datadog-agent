// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/DataDog/datadog-agent/comp/observer/impl/patterns"
)

// PatternKeyInfo contains what can identify a pattern.
type PatternKeyInfo struct {
	// Hash is the ID of this object, used to find it back in the map.
	Hash      int64
	ClusterID int64
}

// NewPatternKeyInfo creates a PatternKeyInfo for the given cluster ID.
func NewPatternKeyInfo(clusterID int64) PatternKeyInfo {
	return PatternKeyInfo{
		Hash:      clusterID + 1,
		ClusterID: clusterID,
	}
}

// LogPatternExtractorConfig holds configuration for the LogPatternExtractor.
type LogPatternExtractorConfig struct{}

// DefaultLogPatternExtractorConfig returns a LogPatternExtractorConfig with default values.
func DefaultLogPatternExtractorConfig() LogPatternExtractorConfig {
	return LogPatternExtractorConfig{}
}

// LogPatternExtractor is a LogMetricsExtractor that clusters log messages into
// patterns and emits a count metric per pattern.
type LogPatternExtractor struct {
	PatternClusterer *patterns.PatternClusterer
	PatternKeys      map[int64]PatternKeyInfo
}

var _ observerdef.LogMetricsExtractor = (*LogPatternExtractor)(nil)

// NewLogPatternExtractor creates a new LogPatternExtractor.
func NewLogPatternExtractor() *LogPatternExtractor {
	return &LogPatternExtractor{
		PatternClusterer: patterns.NewPatternClusterer(patterns.IDComputeInfo{
			Offset: 0,
			Stride: 1,
			Index:  0,
		}),
		PatternKeys: make(map[int64]PatternKeyInfo),
	}
}

// Name returns the extractor name.
func (e *LogPatternExtractor) Name() string {
	return "log_pattern_extractor"
}

// ProcessLog clusters the log message and emits a count metric for its pattern.
func (e *LogPatternExtractor) ProcessLog(log observerdef.LogView) []observerdef.MetricOutput {
	message := string(log.GetContent())
	clusterResult := e.PatternClusterer.Process(message)
	if clusterResult == nil {
		return nil
	}

	patternKey := NewPatternKeyInfo(clusterResult.Cluster.ID)
	e.PatternKeys[patternKey.Hash] = patternKey

	return []observerdef.MetricOutput{{
		Name:  fmt.Sprintf("log.%s.%x.count", e.Name(), patternKey.Hash),
		Value: 1,
		Tags:  log.GetTags(),
	}}
}
