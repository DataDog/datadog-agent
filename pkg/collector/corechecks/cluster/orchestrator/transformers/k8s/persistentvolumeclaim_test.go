// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestExtractPersistentVolumeClaim(t *testing.T) {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))
	filesystem := corev1.PersistentVolumeFilesystem
	parsedResource := resource.MustParse("2Gi")

	tests := map[string]struct {
		input    corev1.PersistentVolumeClaim
		expected model.PersistentVolumeClaim
	}{
		"full pvc": {
			input: corev1.PersistentVolumeClaim{
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
					ResourceVersion: "220593670",
					UID:             types.UID("0ff96226-578d-4679-b3c8-72e8a485c0ef"),
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany, corev1.ReadWriteOnce},
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "test-sts",
						},
					},
					Resources: corev1.ResourceRequirements{
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
			},
			expected: model.PersistentVolumeClaim{
				Metadata: &model.Metadata{
					Annotations:       []string{"annotation:my-annotation"},
					CreationTimestamp: creationTime.Unix(),
					Labels:            []string{"app:my-app"},
					Finalizers:        []string{"foo.com/x", metav1.FinalizerOrphanDependents, "bar.com/y"},
					Name:              "pvc",
					Namespace:         "project",
					ResourceVersion:   "220593670",
					Uid:               "0ff96226-578d-4679-b3c8-72e8a485c0ef",
				},
				Spec: &model.PersistentVolumeClaimSpec{
					AccessModes: []string{string(corev1.ReadWriteMany), string(corev1.ReadWriteOnce)},
					Selector: []*model.LabelSelectorRequirement{
						{
							Key:      "app",
							Operator: "In",
							Values:   []string{"test-sts"},
						},
					},
					Resources:        &model.ResourceRequirements{Requests: map[string]int64{string(corev1.ResourceStorage): parsedResource.Value()}},
					VolumeName:       "elasticsearch-volume",
					StorageClassName: "gold",
					VolumeMode:       string(corev1.PersistentVolumeFilesystem),
					DataSource: &model.TypedLocalObjectReference{
						Name: "srcpvc",
						Kind: "PersistentVolumeClaim",
					},
				},
				Status: &model.PersistentVolumeClaimStatus{
					Phase:       string(corev1.ClaimLost),
					AccessModes: []string{string(corev1.ReadWriteOnce)},
					Capacity:    map[string]int64{string(corev1.ResourceStorage): parsedResource.Value()},
					Conditions: []*model.PersistentVolumeClaimCondition{{
						Reason: "OfflineResize",
					}},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, &tc.expected, ExtractPersistentVolumeClaim(&tc.input))
		})
	}
}
