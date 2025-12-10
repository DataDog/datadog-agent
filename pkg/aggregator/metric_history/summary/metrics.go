// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package summary

// ExtractMetricPattern finds the common prefix among metric names
// and returns the varying suffixes.
//
// Examples:
//
//	[system.disk.free, system.disk.used] -> Family: "system.disk", Variants: ["free", "used"]
//	[system.cpu.user.total, system.cpu.system.total] -> Family: "system.cpu", Variants: ["user.total", "system.total"]
//	[system.load.1, system.load.1] -> Family: "system.load.1", Variants: []
func ExtractMetricPattern(events []AnomalyEvent) MetricPattern {
	// Handle empty events
	if len(events) == 0 {
		return MetricPattern{
			Family:   "",
			Variants: []string{},
		}
	}

	// Extract unique metric names
	uniqueMetrics := make(map[string]bool)
	for _, event := range events {
		uniqueMetrics[event.Metric] = true
	}

	// Convert to slice
	metrics := make([]string, 0, len(uniqueMetrics))
	for metric := range uniqueMetrics {
		metrics = append(metrics, metric)
	}

	// Handle single unique metric (including when all metrics are identical)
	if len(metrics) == 1 {
		return MetricPattern{
			Family:   metrics[0],
			Variants: []string{},
		}
	}

	// Split each metric into parts
	metricParts := make([][]string, len(metrics))
	for i, metric := range metrics {
		metricParts[i] = splitMetric(metric)
	}

	// Find the longest common prefix
	commonPrefixLen := findCommonPrefixLength(metricParts)

	// Build the family from common prefix
	var family string
	if commonPrefixLen > 0 {
		family = joinParts(metricParts[0][:commonPrefixLen])
	}

	// Extract variants (suffixes after common prefix)
	variantsSet := make(map[string]bool)
	for _, parts := range metricParts {
		if commonPrefixLen < len(parts) {
			suffix := joinParts(parts[commonPrefixLen:])
			variantsSet[suffix] = true
		}
	}

	// Convert variants to sorted slice
	variants := make([]string, 0, len(variantsSet))
	for variant := range variantsSet {
		variants = append(variants, variant)
	}

	// Sort variants alphabetically
	sortStrings(variants)

	return MetricPattern{
		Family:   family,
		Variants: variants,
	}
}

// splitMetric splits a metric name by dots
func splitMetric(metric string) []string {
	if metric == "" {
		return []string{}
	}

	parts := []string{}
	current := ""
	for i := 0; i < len(metric); i++ {
		if metric[i] == '.' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(metric[i])
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

// findCommonPrefixLength finds the length of the common prefix across all metric parts
func findCommonPrefixLength(metricParts [][]string) int {
	if len(metricParts) == 0 {
		return 0
	}

	// Start with the length of the first metric
	commonLen := len(metricParts[0])

	// Compare with all other metrics
	for i := 1; i < len(metricParts); i++ {
		// Find common prefix with this metric
		maxLen := commonLen
		if len(metricParts[i]) < maxLen {
			maxLen = len(metricParts[i])
		}

		// Find where they differ
		j := 0
		for j < maxLen && metricParts[0][j] == metricParts[i][j] {
			j++
		}

		// Update common length
		if j < commonLen {
			commonLen = j
		}
	}

	return commonLen
}

// joinParts joins metric parts with dots
func joinParts(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += "." + parts[i]
	}
	return result
}

// sortStrings sorts a slice of strings in place (simple bubble sort)
func sortStrings(strs []string) {
	for i := 0; i < len(strs); i++ {
		for j := i + 1; j < len(strs); j++ {
			if strs[i] > strs[j] {
				strs[i], strs[j] = strs[j], strs[i]
			}
		}
	}
}
