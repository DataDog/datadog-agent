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

// // This contains what can identify a pattern
// type PatternKeyInfo struct {
// 	ClusterID int64
// }

type LogPatternExtractor struct {
	PatternClusterer *patterns.PatternClusterer
	// PatternKeys
}

func NewLogPatternExtractor() *LogPatternExtractor {
	return &LogPatternExtractor{
		PatternClusterer: patterns.NewPatternClusterer(patterns.IDComputeInfo{
			Offset: 0,
			Stride: 1,
			Index:  0,
		}),
	}
}

func (e *LogPatternExtractor) Name() string {
	return "log_pattern_extractor"
}

func (e *LogPatternExtractor) ProcessLog(log observerdef.LogView) []observerdef.MetricOutput {
	fmt.Printf("[cc]Processing log: %s\n", log.GetContent())
	message := string(log.GetContent())
	// Extract pattern
	clusterResult := e.PatternClusterer.Process(message)
	if clusterResult == nil {
		return nil
	}

	// TODO: Create a pattern key

	// Emit metric for the pattern
	return []observerdef.MetricOutput{{
		Name:  fmt.Sprintf("log.%s.%d.count", e.Name(), clusterResult.Cluster.ID),
		Value: 1,
		Tags:  log.GetTags(),
	}}
}
