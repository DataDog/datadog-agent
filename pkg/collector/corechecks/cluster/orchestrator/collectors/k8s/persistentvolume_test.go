// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && orchestrator && test

package k8s

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	model "github.com/DataDog/agent-payload/v5/process"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

func TestPersistentVolumeCollector(t *testing.T) {
	creationTime := CreateTestTime()
	filesystem := corev1.PersistentVolumeFilesystem
	parsedResource := resource.MustParse("2Gi")

	persistentVolume := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
			CreationTimestamp: creationTime,
			Labels: map[string]string{
				"app": "my-app",
			},
			Finalizers:      []string{"foo.com/x", metav1.FinalizerOrphanDependents, "bar.com/y"},
			Name:            "pv",
			Namespace:       "project",
			ResourceVersion: "1217",
			UID:             types.UID("0ff96226-578d-4679-b3c8-72e8a485c0ef"),
		},
		Spec: corev1.PersistentVolumeSpec{
			MountOptions: []string{"ro", "soft"},
			Capacity:     corev1.ResourceList{corev1.ResourceStorage: parsedResource},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				GCEPersistentDisk: &corev1.GCEPersistentDiskVolumeSource{
					PDName:    "GCE",
					FSType:    "GCE",
					Partition: 10,
					ReadOnly:  false,
				},
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany, corev1.ReadWriteOnce},
			ClaimRef: &corev1.ObjectReference{
				Namespace: "test",
				Name:      "test-pv",
			},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			StorageClassName:              "gold",
			VolumeMode:                    &filesystem,
			NodeAffinity: &corev1.VolumeNodeAffinity{
				Required: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "test-key3",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"test-value1", "test-value3"},
								},
							},
							MatchFields: []corev1.NodeSelectorRequirement{
								{
									Key:      "test-key2",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"test-value0", "test-value2"},
								},
							},
						},
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "test-key3",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"test-value1", "test-value3"},
								},
							}},
					},
				},
			},
		},
		Status: corev1.PersistentVolumeStatus{
			Phase:   corev1.VolumePending,
			Message: "test",
			Reason:  "test",
		},
	}

	metadataAsTags := utils.GetMetadataAsTags(mockconfig.New(t))
	collector := NewPersistentVolumeCollector(metadataAsTags)

	config := CollectorTestConfig{
		Resources:                  []runtime.Object{persistentVolume},
		ExpectedMetadataType:       &model.CollectorPersistentVolume{},
		ExpectedResourcesListed:    1,
		ExpectedResourcesProcessed: 1,
		ExpectedMetadataMessages:   1,
		ExpectedManifestMessages:   1,
	}

	RunCollectorTest(t, config, collector)
}
