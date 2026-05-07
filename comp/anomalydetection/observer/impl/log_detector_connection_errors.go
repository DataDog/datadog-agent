// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"strings"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// connectionErrorPatterns are the patterns we look for (all lowercase for case-insensitive matching).
var connectionErrorPatterns = []string{
	"connection refused",
	"connection reset",
	"connection timed out",
	"econnrefused",
	"econnreset",
	"etimedout",
}

// ConnectionErrorExtractorConfig holds configuration for the ConnectionErrorExtractor.
type ConnectionErrorExtractorConfig struct{}

// DefaultConnectionErrorExtractorConfig returns a ConnectionErrorExtractorConfig with default values.
func DefaultConnectionErrorExtractorConfig() ConnectionErrorExtractorConfig {
	return ConnectionErrorExtractorConfig{}
}

// ConnectionErrorExtractor is a log detector that detects connection errors
// and converts them into a connection.errors metric.
// It implements observer.ContextProvider so that anomalies detected on the
// emitted metric can be enriched with the matched pattern and an example log line.
type ConnectionErrorExtractor struct {
	// patternContext tracks the matched pattern and an example log line for
	// each (metric, tags) combination so enrichAnomaly can format a
	// human-readable description instead of a raw tag dump.
	patternContext map[string]observer.MetricContext
}

// Name returns the detector name.
func (c *ConnectionErrorExtractor) Name() string {
	return "connection_error_extractor"
}

// Reset clears the context map; called when the engine resets for replay.
func (c *ConnectionErrorExtractor) Reset() {
	c.patternContext = nil
}

// GetContextByKey implements observer.ContextProvider.
func (c *ConnectionErrorExtractor) GetContextByKey(key string) (observer.MetricContext, bool) {
	if c.patternContext == nil {
		return observer.MetricContext{}, false
	}
	ctx, ok := c.patternContext[key]
	return ctx, ok
}

// ProcessLog checks if a log contains connection error patterns and returns a metric if so.
// Anomaly detection is handled by metrics detection on the count aggregation of the emitted metric.
func (c *ConnectionErrorExtractor) ProcessLog(log observer.LogView) observer.LogMetricsExtractorOutput {
	content := strings.ToLower(string(log.GetContent()))
	tags := log.GetTags()

	for _, pattern := range connectionErrorPatterns {
		if strings.Contains(content, pattern) {
			contextKey := metricContextKey("connection.errors", tags)

			if c.patternContext == nil {
				c.patternContext = make(map[string]observer.MetricContext)
			}
			// Store the matched pattern + a truncated example log line so
			// enrichAnomaly can produce "Log pattern change rate detected:
			//   pattern: connection refused
			//   example: <log line>" instead of a raw tag dump.
			c.patternContext[contextKey] = observer.MetricContext{
				Pattern: pattern,
				Example: truncate(string(log.GetContent()), 160),
				Source:  "connection_error_extractor",
			}

			return observer.LogMetricsExtractorOutput{
				Metrics: []observer.MetricOutput{{
					Name:       "connection.errors",
					Value:      1.0,
					Tags:       tags,
					ContextKey: contextKey,
				}},
			}
		}
	}

	return observer.LogMetricsExtractorOutput{}
}
