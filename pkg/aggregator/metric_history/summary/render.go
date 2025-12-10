// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package summary

// Summarize generates a human-readable summary from a cluster.
// It uses the cluster's pattern (metric family, tag partition) and
// optionally detected symmetry to produce meaningful output.
func Summarize(cluster *AnomalyCluster) ClusterSummary {
	// TODO: implement
	return ClusterSummary{}
}

// SummarizeWithSymmetry generates a summary including symmetry information.
// Call DetectSymmetry separately and pass the result here.
func SummarizeWithSymmetry(cluster *AnomalyCluster, symmetry *SymmetryPattern) ClusterSummary {
	// TODO: implement
	return ClusterSummary{}
}
