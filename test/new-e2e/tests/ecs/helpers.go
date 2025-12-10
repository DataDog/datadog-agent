// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ecs

import (
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// Helper functions to aggregate all metrics/logs from fakeintake and common utilities
// These replace the now-private GetMetrics() and GetLogs() methods

import (
	"strings"
)

func getAllMetrics(client *fakeintake.Client) ([]*aggregator.MetricSeries, error) {
	names, err := client.GetMetricNames()
	if err != nil {
		return nil, err
	}
	var allMetrics []*aggregator.MetricSeries
	for _, name := range names {
		metrics, err := client.FilterMetrics(name)
		if err != nil {
			continue
		}
		allMetrics = append(allMetrics, metrics...)
	}
	return allMetrics, nil
}

func getAllLogs(client *fakeintake.Client) ([]*aggregator.Log, error) {
	services, err := client.GetLogServiceNames()
	if err != nil {
		return nil, err
	}
	var allLogs []*aggregator.Log
	for _, service := range services {
		logs, err := client.FilterLogs(service)
		if err != nil {
			continue
		}
		allLogs = append(allLogs, logs...)
	}
	return allLogs, nil
}

// getKeys returns the keys from a map[string]bool (for logging purposes)
func getKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// getMapKeys returns the keys from a map[string]interface{} (for logging purposes)
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// filterLogsByTag filters logs that have a specific tag with a specific value
func filterLogsByTag(logs []*aggregator.Log, tagKey, tagValue string) []*aggregator.Log {
	var filtered []*aggregator.Log
	expectedTag := tagKey + ":" + tagValue
	for _, log := range logs {
		for _, tag := range log.GetTags() {
			if tag == expectedTag || strings.HasPrefix(tag, expectedTag+",") {
				filtered = append(filtered, log)
				break
			}
		}
	}
	return filtered
}

// getTagValue extracts the value from a tag string like "key:value"
func getTagValue(tags []string, key string) string {
	prefix := key + ":"
	for _, tag := range tags {
		if strings.HasPrefix(tag, prefix) {
			return strings.TrimPrefix(tag, prefix)
		}
	}
	return ""
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
