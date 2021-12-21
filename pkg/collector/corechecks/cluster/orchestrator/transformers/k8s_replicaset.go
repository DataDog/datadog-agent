// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package transformers

import (
	model "github.com/DataDog/agent-payload/v5/process"
	appsv1 "k8s.io/api/apps/v1"
)

// ExtractK8sReplicaSet returns the protobuf model corresponding to a Kubernetes
// ReplicaSet resource.
func ExtractK8sReplicaSet(rs *appsv1.ReplicaSet) *model.ReplicaSet {
	replicaSet := model.ReplicaSet{
		Metadata: extractMetadata(&rs.ObjectMeta),
	}
	// spec
	replicaSet.ReplicasDesired = 1 // default
	if rs.Spec.Replicas != nil {
		replicaSet.ReplicasDesired = *rs.Spec.Replicas
	}
	if rs.Spec.Selector != nil {
		replicaSet.Selectors = extractLabelSelector(rs.Spec.Selector)
	}

	// status
	replicaSet.Replicas = rs.Status.Replicas
	replicaSet.FullyLabeledReplicas = rs.Status.FullyLabeledReplicas
	replicaSet.ReadyReplicas = rs.Status.ReadyReplicas
	replicaSet.AvailableReplicas = rs.Status.AvailableReplicas

	return &replicaSet
}
