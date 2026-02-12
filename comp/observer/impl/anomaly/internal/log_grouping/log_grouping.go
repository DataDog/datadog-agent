// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package log_grouping provides log error grouping and clustering functionality.
package log_grouping

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/observer/impl/anomaly/internal/ml_pattern"
	"github.com/DataDog/datadog-agent/comp/observer/impl/anomaly/internal/types"
)

// LogGrouper handles grouping and clustering of log errors
type LogGrouper struct {
	eps    float64 // Distance threshold for DBSCAN (default: 0.3)
	minPts int     // Minimum points to form a cluster (default: 2)
}

// NewLogGrouper creates a new LogGrouper with default settings
func NewLogGrouper() *LogGrouper {
	return &LogGrouper{
		eps:    0.3, // 70%+ similarity required
		minPts: 2,   // At least 2 logs must be similar
	}
}

// GroupWithDBSCAN applies DBSCAN clustering to group similar log errors
func (lg *LogGrouper) GroupWithDBSCAN(logErrors []types.LogError) []types.LogError {
	// Create ML detector with default settings
	mlDetector := ml_pattern.NewMLPatternDetector()

	// Apply DBSCAN clustering
	patterns := mlDetector.DetectPatternsDBSCAN(logErrors, lg.eps, lg.minPts)

	// Convert patterns back to LogError format
	var groupedLogs []types.LogError
	for _, pattern := range patterns {
		// Use the most representative example as the message
		message := pattern.Pattern
		if len(pattern.Examples) > 0 {
			// Use first example as the representative message
			message = pattern.Examples[0]
		}

		groupedLogs = append(groupedLogs, types.LogError{
			Message:   message,
			Count:     pattern.Count,
			Timestamp: time.Now(), // Use current time for grouped patterns
		})
	}

	return groupedLogs
}
