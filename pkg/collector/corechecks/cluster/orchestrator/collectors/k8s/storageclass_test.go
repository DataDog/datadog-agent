// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package k8s

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	model "github.com/DataDog/agent-payload/v5/process"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestStorageClassCollector(t *testing.T) {
	creationTime := CreateTestTime()

	storageClass := &storagev1.StorageClass{
		AllowedTopologies: []corev1.TopologySelectorTerm{
			{
				MatchLabelExpressions: []corev1.TopologySelectorLabelRequirement{
					{
						Key: "topology.kubernetes.io/zone",
						Values: []string{
							"us-central-1a",
							"us-central-1b",
						},
					},
				},
			},
			{
				MatchLabelExpressions: []corev1.TopologySelectorLabelRequirement{
					{
						Key:    "topology.kubernetes.io/region",
						Values: []string{"us-central1"},
					},
				},
			},
		},
		AllowVolumeExpansion: pointer.Ptr(true),
		MountOptions:         []string{"mount-option"},
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
			CreationTimestamp: creationTime,
			Labels: map[string]string{
				"app": "my-app",
			},
			Name:            "storage-class",
			Namespace:       "",
			ResourceVersion: "1226",
			UID:             "c63d0b05-a340-46a6-9a38-1161ff1f9bea",
		},
		Parameters: map[string]string{
			"foo": "bar",
		},
		Provisioner:       "provisioner",
		ReclaimPolicy:     pointer.Ptr(corev1.PersistentVolumeReclaimRetain),
		VolumeBindingMode: pointer.Ptr(storagev1.VolumeBindingWaitForFirstConsumer),
	}

	metadataAsTags := utils.GetMetadataAsTags(mockconfig.New(t))
	collector := NewStorageClassCollector(metadataAsTags)

	config := CollectorTestConfig{
		Resources:                  []runtime.Object{storageClass},
		ExpectedMetadataType:       &model.CollectorStorageClass{},
		ExpectedResourcesListed:    1,
		ExpectedResourcesProcessed: 1,
		ExpectedMetadataMessages:   1,
		ExpectedManifestMessages:   1,
	}

	RunCollectorTest(t, config, collector)
}
