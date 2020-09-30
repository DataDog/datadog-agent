// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver, orchestrator

package orchestrator

import (
	"path"

	"github.com/DataDog/datadog-agent/pkg/orchestrator"
)

var (
	StatsKey = "orchestrator/last/run/stats"
)

// CheckStats holds statistics for the DCA status command regarding the last run check. Information is saved in the KubernetesResourceCache.
type CheckStats struct {
	// CacheHits contains the number of cache hits for a NodeType per run.
	CacheHits int

	// CacheMiss contains the number of cache miss/send Data for a NodeType per run.
	CacheMiss int

	orchestrator.NodeType
}

func BuildStatsKey(nodeType orchestrator.NodeType) string {
	keys := append([]string{StatsKey}, nodeType.String())
	return path.Join(keys...)
}
