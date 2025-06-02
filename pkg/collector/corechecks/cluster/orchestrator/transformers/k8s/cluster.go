// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

//nolint:revive // TODO(CAPP) Fix revive linter
package k8s

import (
	corev1 "k8s.io/api/core/v1"

	model "github.com/DataDog/agent-payload/v5/process"
)

const (
	labelInstanceType           = "node.kubernetes.io/instance-type"
	labelInstanceTypeDeprecated = "beta.kubernetes.io/instance-type"
	labelRegion                 = "topology.kubernetes.io/region"
	labelRegionDeprecated       = "failure-domain.beta.kubernetes.io/region"
)

// ExtractClusterNodeInfo extracts a summary of node information.
func ExtractClusterNodeInfo(n *corev1.Node) *model.ClusterNodeInfo {
	return &model.ClusterNodeInfo{
		Architecture:            n.Status.NodeInfo.Architecture,
		ContainerRuntimeVersion: n.Status.NodeInfo.ContainerRuntimeVersion,
		KernelVersion:           n.Status.NodeInfo.KernelVersion,
		KubeletVersion:          n.Status.NodeInfo.KubeletVersion,
		InstanceType:            getWellKnownLabel(n, labelInstanceType, labelInstanceTypeDeprecated),
		Name:                    n.Name,
		OperatingSystem:         n.Status.NodeInfo.OperatingSystem,
		OperatingSystemImage:    n.Status.NodeInfo.OSImage,
		Region:                  getWellKnownLabel(n, labelRegion, labelRegionDeprecated),
		ResourceAllocatable:     toResourceQuantityMap(n.Status.Allocatable),
		ResourceCapacity:        toResourceQuantityMap(n.Status.Capacity),
	}
}

func getWellKnownLabel(node *corev1.Node, label, labelDeprecated string) string {
	if instanceType, ok := node.Labels[label]; ok {
		return instanceType
	}
	if instanceType, ok := node.Labels[labelDeprecated]; ok {
		return instanceType
	}
	return ""
}

func toResourceQuantityMap(resourceList corev1.ResourceList) map[string]string {
	quantities := make(map[string]string)
	for name, quantity := range resourceList {
		quantities[string(name)] = quantity.String()
	}
	return quantities
}
