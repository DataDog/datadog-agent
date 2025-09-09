// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package store

import (
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/kube-state-metrics/v2/pkg/metric"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/ksm/customresources"
)

// RolloutMetricsStore extends MetricsStore with rollout-specific delete handling
type RolloutMetricsStore struct {
	*MetricsStore
	resourceType string
}

// NewRolloutMetricsStore creates a MetricsStore with rollout delete notifications
func NewRolloutMetricsStore(generateFunc func(interface{}) []metric.FamilyInterface, mt string, resourceType string) *RolloutMetricsStore {
	return &RolloutMetricsStore{
		MetricsStore: NewMetricsStore(generateFunc, mt),
		resourceType: resourceType,
	}
}

// Delete overrides the base Delete to add rollout cleanup
func (s *RolloutMetricsStore) Delete(obj interface{}) error {
	// Handle rollout cleanup based on resource type
	switch s.resourceType {
	case "deployments":
		if dep, ok := obj.(*appsv1.Deployment); ok {
			customresources.CleanupDeployment(dep.Namespace, dep.Name)
		}
	case "replicasets":
		if rs, ok := obj.(*appsv1.ReplicaSet); ok {
			customresources.CleanupDeletedReplicaSet(rs.Namespace, rs.Name)
		}
	}

	// Call the base delete method
	return s.MetricsStore.Delete(obj)
}
