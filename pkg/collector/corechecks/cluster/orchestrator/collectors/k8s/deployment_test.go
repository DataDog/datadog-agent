// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && orchestrator && test

package k8s

import (
	"testing"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	model "github.com/DataDog/agent-payload/v5/process"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

func TestDeploymentCollector(t *testing.T) {
	testIntOrStrPercent := intstr.FromString("1%")
	timestamp := metav1.NewTime(CreateTestTime().Time)
	testInt32 := int32(2)

	deployment := &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			UID:               types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
			Name:              "deploy",
			Namespace:         "namespace",
			CreationTimestamp: timestamp,
			Labels: map[string]string{
				"label": "foo",
			},
			Annotations: map[string]string{
				"annotation": "bar",
			},
			ResourceVersion: "1209",
		},
		Spec: v1.DeploymentSpec{
			MinReadySeconds:         600,
			ProgressDeadlineSeconds: &testInt32,
			Replicas:                &testInt32,
			RevisionHistoryLimit:    &testInt32,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test-deploy",
				},
			},
			Strategy: v1.DeploymentStrategy{
				Type: v1.DeploymentStrategyType("RollingUpdate"),
				RollingUpdate: &v1.RollingUpdateDeployment{
					MaxSurge:       &testIntOrStrPercent,
					MaxUnavailable: &testIntOrStrPercent,
				},
			},
		},
		Status: v1.DeploymentStatus{
			AvailableReplicas:  2,
			ObservedGeneration: 3,
			ReadyReplicas:      2,
			Replicas:           2,
			UpdatedReplicas:    2,
			Conditions: []v1.DeploymentCondition{
				{
					Type:    v1.DeploymentAvailable,
					Status:  corev1.ConditionFalse,
					Reason:  "MinimumReplicasAvailable",
					Message: "Deployment has minimum availability.",
				},
				{
					Type:    v1.DeploymentProgressing,
					Status:  corev1.ConditionFalse,
					Reason:  "NewReplicaSetAvailable",
					Message: `ReplicaSet "orchestrator-intake-6d65b45d4d" has timed out progressing.`,
				},
			},
		},
	}

	metadataAsTags := utils.GetMetadataAsTags(mockconfig.New(t))
	collector := NewDeploymentCollector(metadataAsTags)

	config := CollectorTestConfig{
		Resources:                  []runtime.Object{deployment},
		ExpectedMetadataType:       &model.CollectorDeployment{},
		ExpectedResourcesListed:    1,
		ExpectedResourcesProcessed: 1,
		ExpectedMetadataMessages:   1,
		ExpectedManifestMessages:   1,
	}

	RunCollectorTest(t, config, collector)
}
