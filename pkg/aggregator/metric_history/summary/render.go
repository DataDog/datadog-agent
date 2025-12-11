// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package summary

import (
	"fmt"
	"sort"
	"strings"
)

// Summarize generates a human-readable summary from a cluster.
// It uses the cluster's pattern (metric family, tag partition) and
// optionally detected symmetry to produce meaningful output.
func Summarize(cluster *AnomalyCluster) ClusterSummary {
	return SummarizeWithSymmetry(cluster, nil)
}

// SummarizeWithSymmetry generates a summary including symmetry information.
// Call DetectSymmetry separately and pass the result here.
func SummarizeWithSymmetry(cluster *AnomalyCluster, symmetry *SymmetryPattern) ClusterSummary {
	return ClusterSummary{
		Headline:    buildHeadline(cluster, symmetry),
		Details:     buildDetails(cluster, symmetry),
		LikelyCause: inferLikelyCause(cluster, symmetry),
	}
}

// buildHeadline constructs the main summary line
func buildHeadline(cluster *AnomalyCluster, symmetry *SymmetryPattern) string {
	// Base: metric family
	headline := cluster.Pattern.Family

	// Add change type based on symmetry or event direction
	if symmetry != nil && symmetry.Type == Inverse {
		headline += " changed" // or "shift"
	} else {
		headline += " changed"
	}

	// Add dimension from varying tags
	// e.g., "across 6 devices" or "on host web-01"
	for tagKey, values := range cluster.Pattern.VaryingTags {
		if len(values) > 1 {
			headline += fmt.Sprintf(" across %d %ss", len(values), tagKey)
			break // just use first varying dimension
		} else if len(values) == 1 {
			headline += fmt.Sprintf(" on %s %s", tagKey, values[0])
			break
		}
	}

	return headline
}

// buildDetails constructs the bullet points
func buildDetails(cluster *AnomalyCluster, symmetry *SymmetryPattern) []string {
	var details []string

	// Symmetry pattern
	if symmetry != nil {
		switch symmetry.Type {
		case Inverse:
			// "Pattern: free↑ = used↓ (inverse)"
			details = append(details, fmt.Sprintf("Pattern: %s↑ = %s↓ (inverse)",
				shortName(symmetry.Metrics[0]), shortName(symmetry.Metrics[1])))
		case Proportional:
			details = append(details, fmt.Sprintf("Pattern: %s ~ %s (proportional)",
				shortName(symmetry.Metrics[0]), shortName(symmetry.Metrics[1])))
		}
	}

	// Metric variants (if multiple)
	if len(cluster.Pattern.Variants) > 1 {
		// Sort variants for consistent output
		sortedVariants := make([]string, len(cluster.Pattern.Variants))
		copy(sortedVariants, cluster.Pattern.Variants)
		sort.Strings(sortedVariants)
		details = append(details, "Metrics: "+strings.Join(sortedVariants, ", "))
	}

	// Varying tags with values
	// Sort keys for consistent order
	var varyingKeys []string
	for tagKey := range cluster.Pattern.VaryingTags {
		varyingKeys = append(varyingKeys, tagKey)
	}
	sort.Strings(varyingKeys)

	for _, tagKey := range varyingKeys {
		values := cluster.Pattern.VaryingTags[tagKey]
		// Sort values for consistent output
		sortedValues := make([]string, len(values))
		copy(sortedValues, values)
		sort.Strings(sortedValues)

		valStr := strings.Join(sortedValues[:min(3, len(sortedValues))], ", ")
		if len(sortedValues) > 3 {
			valStr += ", ..."
		}
		details = append(details, fmt.Sprintf("Affected %ss: %s", tagKey, valStr))
	}

	// Constant tags (Environment)
	if len(cluster.Pattern.ConstantTags) > 0 {
		// Sort keys for consistent order
		var constantKeys []string
		for k := range cluster.Pattern.ConstantTags {
			constantKeys = append(constantKeys, k)
		}
		sort.Strings(constantKeys)

		var tags []string
		for _, k := range constantKeys {
			v := cluster.Pattern.ConstantTags[k]
			tags = append(tags, k+"="+v)
		}
		details = append(details, "Environment: "+strings.Join(tags, ", "))
	}

	// Event count
	details = append(details, fmt.Sprintf("Events: %d", len(cluster.Events)))

	return details
}

// inferLikelyCause attempts to provide a heuristic explanation
func inferLikelyCause(cluster *AnomalyCluster, symmetry *SymmetryPattern) string {
	family := cluster.Pattern.Family

	// Disk with inverse symmetry + multiple volumes
	if strings.Contains(family, "disk") && symmetry != nil && symmetry.Type == Inverse {
		if deviceCount := len(cluster.Pattern.VaryingTags["device"]); deviceCount > 2 {
			return "APFS volume group operation (snapshot, reclamation, or Time Machine)"
		}
	}

	// CPU spike
	if strings.Contains(family, "cpu") {
		// Could check for spike pattern
		return "Process CPU burst"
	}

	// Memory pressure
	if strings.Contains(family, "mem") {
		return "Memory pressure"
	}

	// Unknown - return empty, don't guess
	return ""
}

// shortName extracts the last part of a metric name
// "system.disk.free" -> "free"
func shortName(metric string) string {
	parts := strings.Split(metric, ".")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return metric
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
