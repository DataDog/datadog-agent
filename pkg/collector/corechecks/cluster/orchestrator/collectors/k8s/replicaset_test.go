// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && orchestrator && test

package k8s

import (
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	model "github.com/DataDog/agent-payload/v5/process"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

func TestReplicaSetCollector(t *testing.T) {
	timestamp := metav1.NewTime(time.Date(2014, time.January, 15, 0, 0, 0, 0, time.UTC)) // 1389744000
	testInt32 := int32(2)

	replicaSet := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			UID:               types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
			Name:              "replicaset",
			Namespace:         "namespace",
			CreationTimestamp: timestamp,
			Labels: map[string]string{
				"label": "foo",
			},
			Annotations: map[string]string{
				"annotation": "bar",
			},
			ResourceVersion: "1220",
		},
		Spec: appsv1.ReplicaSetSpec{
			Replicas: &testInt32,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test-deploy",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "cluster",
						Operator: "NotIn",
						Values:   []string{"staging", "prod"},
					},
				},
			},
		},
		Status: appsv1.ReplicaSetStatus{
			Replicas:             2,
			FullyLabeledReplicas: 2,
			ReadyReplicas:        1,
			AvailableReplicas:    1,
			Conditions: []appsv1.ReplicaSetCondition{
				{
					Type:               appsv1.ReplicaSetReplicaFailure,
					Status:             v1.ConditionFalse,
					LastTransitionTime: timestamp,
					Reason:             "test reason",
					Message:            "test message",
				},
			},
		},
	}

	metadataAsTags := utils.GetMetadataAsTags(mockconfig.New(t))
	collector := NewReplicaSetCollector(metadataAsTags)

	config := CollectorTestConfig{
		Resources:                  []runtime.Object{replicaSet},
		ExpectedMetadataType:       &model.CollectorReplicaSet{},
		ExpectedResourcesListed:    1,
		ExpectedResourcesProcessed: 1,
		ExpectedMetadataMessages:   1,
		ExpectedManifestMessages:   1,
	}

	RunCollectorTest(t, config, collector)
}
