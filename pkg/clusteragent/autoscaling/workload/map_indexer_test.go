// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"testing"

	"github.com/stretchr/testify/assert"
	autoscalingv2 "k8s.io/api/autoscaling/v2"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
)

func TestMapIndexer(t *testing.T) {
	indexer := newMapIndexer()

	// Check adding works
	ownerRef := model.OwnerReference{
		Namespace:  "test",
		Name:       "test",
		Kind:       "PodAutoscaler",
		APIVersion: "v1alpha2",
	}

	ownerRef2 := model.OwnerReference{
		Namespace:  "test2",
		Name:       "test2",
		Kind:       "PodAutoscaler",
		APIVersion: "v1alpha2",
	}

	// Check adding works
	indexer.AddToIndex(ownerRef, "test/test")
	assert.Equal(t, []string{"test/test"}, indexer.GetIDs(ownerRef))
	indexer.AddToIndex(ownerRef2, "test2/test2")
	assert.Equal(t, []string{"test2/test2"}, indexer.GetIDs(ownerRef2))

	// Check adding to the same key works
	indexer.AddToIndex(ownerRef, "test/test2")
	assert.Equal(t, []string{"test/test", "test/test2"}, indexer.GetIDs(ownerRef))

	// Check removing works
	indexer.RemoveFromIndex(ownerRef, "test/test")
	assert.Equal(t, []string{"test/test2"}, indexer.GetIDs(ownerRef))

	// Check removing a	non-existing ID does not error
	indexer.RemoveFromIndex(ownerRef, "test/test")
	assert.Equal(t, []string{"test/test2"}, indexer.GetIDs(ownerRef))

	// Check removing the last ID from the key works
	indexer.RemoveFromIndex(ownerRef, "test/test2")
	assert.Equal(t, []string{}, indexer.GetIDs(ownerRef))
}

func TestMapIndexer_GetIndexKey(t *testing.T) {
	indexer := newMapIndexer()

	podAutoscaler := model.NewFakePodAutoscalerInternal("test", "test", &model.FakePodAutoscalerInternal{
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind:       "PodAutoscaler",
				Name:       "test",
				APIVersion: "v1alpha2",
			},
		},
	})

	assert.Equal(t, model.OwnerReference{
		Namespace:  "test",
		Name:       "test",
		Kind:       "PodAutoscaler",
		APIVersion: "v1alpha2",
	}, indexer.GetIndexKey(podAutoscaler))
}
