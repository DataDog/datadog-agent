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

	model "github.com/DataDog/agent-payload/v5/process"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

func TestStatefulSetCollector(t *testing.T) {
	creationTime := CreateTestTime()
	testInt32 := int32(3)

	statefulSet := &v1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
			CreationTimestamp: creationTime,
			Labels: map[string]string{
				"app": "my-app",
			},
			Finalizers:      []string{"foo.com/x", metav1.FinalizerOrphanDependents, "bar.com/y"},
			Name:            "statefulset",
			Namespace:       "namespace",
			ResourceVersion: "1225",
			UID:             types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
		},
		Spec: v1.StatefulSetSpec{
			Replicas: &testInt32,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test-sts",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "test-sts",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginx:1.19",
						},
					},
				},
			},
			ServiceName:          "test-sts-service",
			PodManagementPolicy:  v1.ParallelPodManagement,
			UpdateStrategy:       v1.StatefulSetUpdateStrategy{Type: v1.RollingUpdateStatefulSetStrategyType},
			RevisionHistoryLimit: &testInt32,
		},
		Status: v1.StatefulSetStatus{
			ObservedGeneration: 1,
			Replicas:           3,
			ReadyReplicas:      3,
			CurrentReplicas:    3,
			UpdatedReplicas:    3,
			CurrentRevision:    "statefulset-1",
			UpdateRevision:     "statefulset-1",
			CollisionCount:     &testInt32,
		},
	}

	metadataAsTags := utils.GetMetadataAsTags(mockconfig.New(t))
	collector := NewStatefulSetCollector(metadataAsTags)

	config := CollectorTestConfig{
		Resources:                  []runtime.Object{statefulSet},
		ExpectedMetadataType:       &model.CollectorStatefulSet{},
		ExpectedResourcesListed:    1,
		ExpectedResourcesProcessed: 1,
		ExpectedMetadataMessages:   1,
		ExpectedManifestMessages:   1,
	}

	RunCollectorTest(t, config, collector)
}
