// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

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
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestPersistentVolumeClaimCollector(t *testing.T) {
	creationTime := CreateTestTime()
	filesystem := corev1.PersistentVolumeFilesystem

	persistentVolumeClaim := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
			CreationTimestamp: creationTime,
			Labels: map[string]string{
				"app": "my-app",
			},
			Finalizers:      []string{"foo.com/x", metav1.FinalizerOrphanDependents, "bar.com/y"},
			Name:            "pvc",
			Namespace:       "project",
			ResourceVersion: "1218",
			UID:             types.UID("0ff96226-578d-4679-b3c8-72e8a485c0ef"),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany, corev1.ReadWriteOnce},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test-sts",
				},
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("2Gi"),
				},
			},
			VolumeName:       "elasticsearch-volume",
			StorageClassName: pointer.Ptr("gold"),
			VolumeMode:       &filesystem,
			DataSource: &corev1.TypedLocalObjectReference{
				Name: "srcpvc",
				Kind: "PersistentVolumeClaim",
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase:       corev1.ClaimLost,
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("2Gi"),
			},
			Conditions: []corev1.PersistentVolumeClaimCondition{
				{Reason: "OfflineResize"},
			},
		},
	}

	metadataAsTags := utils.GetMetadataAsTags(mockconfig.New(t))
	collector := NewPersistentVolumeClaimCollector(metadataAsTags)

	config := CollectorTestConfig{
		Resources:                  []runtime.Object{persistentVolumeClaim},
		ExpectedMetadataType:       &model.CollectorPersistentVolumeClaim{},
		ExpectedResourcesListed:    1,
		ExpectedResourcesProcessed: 1,
		ExpectedMetadataMessages:   1,
		ExpectedManifestMessages:   1,
	}

	RunCollectorTest(t, config, collector)
}
