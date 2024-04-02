// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"

	model "github.com/DataDog/agent-payload/v5/process"
)

// ExtractStorageClass returns the protobuf model corresponding to a Kubernetes StorageClass resource.
func ExtractStorageClass(sc *storagev1.StorageClass) *model.StorageClass {
	msg := &model.StorageClass{
		AllowedTopologies: extractStorageClassTopologies(sc.AllowedTopologies),
		Metadata:          extractMetadata(&sc.ObjectMeta),
		MountOptions:      sc.MountOptions,
		Parameters:        sc.Parameters,
		Provisionner:      sc.Provisioner,
	}

	// Defaults
	msg.AllowVolumeExpansion = false
	msg.ReclaimPolicy = string(corev1.PersistentVolumeReclaimDelete)
	msg.VolumeBindingMode = string(storagev1.VolumeBindingImmediate)

	if sc.AllowVolumeExpansion != nil {
		msg.AllowVolumeExpansion = bool(*sc.AllowVolumeExpansion)
	}
	if sc.ReclaimPolicy != nil {
		msg.ReclaimPolicy = string(*sc.ReclaimPolicy)
	}
	if sc.VolumeBindingMode != nil {
		msg.VolumeBindingMode = string(*sc.VolumeBindingMode)
	}

	return msg
}

func extractStorageClassTopologies(topologies []corev1.TopologySelectorTerm) []*model.StorageClassTopology {
	t := make([]*model.StorageClassTopology, 0, len(topologies))
	for _, topology := range topologies {
		t = append(t, extractStorageClassTopology(topology))
	}
	return t
}

func extractStorageClassTopology(topology corev1.TopologySelectorTerm) *model.StorageClassTopology {
	var t model.StorageClassTopology
	for _, labelSelector := range topology.MatchLabelExpressions {
		t.TopologySelectors = append(t.TopologySelectors, &model.TopologyLabelSelector{
			Key:    labelSelector.Key,
			Values: labelSelector.Values,
		})
	}
	return &t
}
