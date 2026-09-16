// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"slices"
	"testing"
)

func TestMinimumTagFiltersForClusters(t *testing.T) {
	metadata := map[string]*clusterAgentMetadata{
		"target-a": {tags: map[string]map[string]struct{}{
			"datacenter":        {"dc-a": {}},
			"kube_cluster_name": {"target-a": {}},
		}},
		"target-b": {tags: map[string]map[string]struct{}{
			"datacenter":        {"dc-b": {}},
			"kube_cluster_name": {"target-b": {}},
		}},
		"target-c": {tags: map[string]map[string]struct{}{
			"datacenter":        {"dc-b": {}},
			"kube_cluster_name": {"target-c": {}},
		}},
		"excluded": {tags: map[string]map[string]struct{}{
			"datacenter": {"dc-a": {}},
		}},
	}

	got := minimumTagFiltersForClusters(metadata, []string{"target-a", "target-b", "target-c"})
	want := []string{"datacenter:dc-b", "kube_cluster_name:target-a"}
	if !slices.Equal(got, want) {
		t.Fatalf("minimumTagFiltersForClusters() = %v, want %v", got, want)
	}
}
