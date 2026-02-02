// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build observer

package observerimpl

import (
	"strings"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
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

// ConnectionErrorExtractor is a log processor that detects connection errors
// and converts them into a connection.errors metric.
type ConnectionErrorExtractor struct{}

// Name returns the processor name.
func (c *ConnectionErrorExtractor) Name() string {
	return "connection_error_extractor"
}

// Process checks if a log contains connection error patterns and returns a metric if so.
// Anomaly detection is handled by TS analysis on the count aggregation of the emitted metric.
func (c *ConnectionErrorExtractor) Process(log observer.LogView) observer.LogProcessorResult {
	content := strings.ToLower(string(log.GetContent()))

	for _, pattern := range connectionErrorPatterns {
		if strings.Contains(content, pattern) {
			return observer.LogProcessorResult{
				Metrics: []observer.MetricOutput{{
					Name:  "connection.errors",
					Value: 1.0,
					Tags:  log.GetTags(),
				}},
			}
		}
	}

	return observer.LogProcessorResult{}
}
