// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"slices"
	"testing"

	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
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

func TestAppendTagInventoryFilter(t *testing.T) {
	got := appendTagInventoryFilter(
		[]string{"datacenter:dc-b", "kube_cluster_name:target-a"},
		"env:production",
	)
	want := []string{
		"datacenter:dc-b,env:production",
		"kube_cluster_name:target-a,env:production",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("appendTagInventoryFilter() = %v, want %v", got, want)
	}
}

func TestTagInventoryFiltersKeepKubernetesScopePerExtraFilter(t *testing.T) {
	config := gpuspec.GPUConfig{
		Architecture: "ampere",
		DeviceMode:   gpuspec.DeviceModePhysical,
	}

	got := tagInventoryFiltersForConfig(config, []string{
		"datacenter:dc-b",
		"kube_cluster_name:target-a",
	})
	want := []string{
		"gpu_architecture:ampere,kube_cluster_name:*,gpu_slicing_mode:none,gpu_virtualization_mode:none,datacenter:dc-b",
		"gpu_architecture:ampere,kube_cluster_name:*,gpu_slicing_mode:none,gpu_virtualization_mode:passthrough,datacenter:dc-b",
		"gpu_architecture:ampere,gpu_slicing_mode:none,gpu_virtualization_mode:none,kube_cluster_name:target-a",
		"gpu_architecture:ampere,gpu_slicing_mode:none,gpu_virtualization_mode:passthrough,kube_cluster_name:target-a",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("tagInventoryFiltersForConfig() = %v, want %v", got, want)
	}
}
