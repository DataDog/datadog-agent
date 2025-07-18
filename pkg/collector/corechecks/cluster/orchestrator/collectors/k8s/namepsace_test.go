// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package k8s

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	model "github.com/DataDog/agent-payload/v5/process"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

func TestNamespaceCollector(t *testing.T) {
	creationTime := CreateTestTime()

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
			CreationTimestamp: creationTime,
			Labels: map[string]string{
				"app": "my-app",
			},
			Name:            "my-name",
			Namespace:       "my-namespace",
			ResourceVersion: "1214",
			Finalizers:      []string{"final", "izers"},
			UID:             types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
		},
		Status: corev1.NamespaceStatus{
			Phase: "a-phase",
			Conditions: []corev1.NamespaceCondition{
				{
					Type:    "NamespaceFinalizersRemaining",
					Status:  "False",
					Message: "wrong msg",
				},
				{
					Type:    "NamespaceDeletionContentFailure",
					Status:  "True",
					Message: "also the wrong msg",
				},
				{
					Type:    "NamespaceDeletionDiscoveryFailure",
					Status:  "True",
					Message: "right msg",
				},
			},
		},
	}

	metadataAsTags := utils.GetMetadataAsTags(mockconfig.New(t))
	collector := NewNamespaceCollector(metadataAsTags)

	config := CollectorTestConfig{
		Resources:                  []runtime.Object{namespace},
		ExpectedMetadataType:       &model.CollectorNamespace{},
		ExpectedResourcesListed:    1,
		ExpectedResourcesProcessed: 1,
		ExpectedMetadataMessages:   1,
		ExpectedManifestMessages:   1,
	}

	RunCollectorTest(t, config, collector)
}
