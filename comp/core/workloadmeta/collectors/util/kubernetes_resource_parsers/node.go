// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesresourceparsers

import (
	corev1 "k8s.io/api/core/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

type nodeParser struct{}

// NewNodeParser returns an ObjectParser for corev1.Node objects.
func NewNodeParser() ObjectParser {
	return nodeParser{}
}

func (p nodeParser) Parse(obj interface{}) workloadmeta.Entity {
	node := obj.(*corev1.Node)
	return &workloadmeta.KubernetesNode{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesNode,
			ID:   node.Name,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        node.Name,
			Labels:      node.Labels,
			Annotations: node.Annotations,
			UID:         string(node.UID),
		},
		Status: workloadmeta.KubernetesNodeStatus{
			KubeletVersion:          node.Status.NodeInfo.KubeletVersion,
			KernelVersion:           node.Status.NodeInfo.KernelVersion,
			OSImage:                 node.Status.NodeInfo.OSImage,
			ContainerRuntimeVersion: node.Status.NodeInfo.ContainerRuntimeVersion,
			Architecture:            node.Status.NodeInfo.Architecture,
			OperatingSystem:         node.Status.NodeInfo.OperatingSystem,
		},
	}
}
