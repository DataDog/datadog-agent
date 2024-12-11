// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"strings"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var (
	creationTime   = metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))
	filesystem     = corev1.PersistentVolumeFilesystem
	parsedResource = resource.MustParse("2Gi")
)

func TestExtractPersistentVolume(t *testing.T) {
	basicInputPV := newInputPV()
	basicExpectedPV := newExpectedPV()

	tests := map[string]struct {
		basicInputPV    corev1.PersistentVolume
		inputSource     corev1.PersistentVolumeSource
		basicExpectedPV model.PersistentVolume
		expectedSource  *model.PersistentVolumeSource
		expectedType    string
	}{
		"full pv": {
			basicInputPV:    basicInputPV,
			basicExpectedPV: basicExpectedPV,
			expectedType:    "<unknown>",
		},
		"gce": {
			basicInputPV: basicInputPV,
			inputSource: corev1.PersistentVolumeSource{
				GCEPersistentDisk: &corev1.GCEPersistentDiskVolumeSource{
					PDName:    "GCE",
					FSType:    "GCE",
					Partition: 10,
					ReadOnly:  false,
				},
			},
			basicExpectedPV: basicExpectedPV,
			expectedSource: &model.PersistentVolumeSource{
				GcePersistentDisk: &model.GCEPersistentDiskVolumeSource{
					PdName:    "GCE",
					FsType:    "GCE",
					Partition: 10,
					ReadOnly:  false,
				},
			},
			expectedType: "GCEPersistentDisk",
		},
		"aws": {
			basicInputPV: basicInputPV,
			inputSource: corev1.PersistentVolumeSource{
				AWSElasticBlockStore: &corev1.AWSElasticBlockStoreVolumeSource{
					VolumeID:  "abc",
					FSType:    "aws",
					Partition: 10,
					ReadOnly:  false,
				},
			},
			basicExpectedPV: basicExpectedPV,
			expectedSource: &model.PersistentVolumeSource{
				AwsElasticBlockStore: &model.AWSElasticBlockStoreVolumeSource{
					VolumeID:  "abc",
					FsType:    "aws",
					Partition: 10,
					ReadOnly:  false,
				},
			},
			expectedType: "AWSElasticBlockStore",
		},
		"azure file": {
			basicInputPV: basicInputPV,
			inputSource: corev1.PersistentVolumeSource{
				AzureFile: &corev1.AzureFilePersistentVolumeSource{
					SecretName:      "secret",
					ShareName:       "share",
					ReadOnly:        false,
					SecretNamespace: toPt("default"),
				},
			},
			basicExpectedPV: basicExpectedPV,
			expectedSource: &model.PersistentVolumeSource{
				AzureFile: &model.AzureFilePersistentVolumeSource{
					SecretName:      "secret",
					ShareName:       "share",
					ReadOnly:        false,
					SecretNamespace: "default",
				},
			},
			expectedType: "AzureFile",
		},
		"azure disk": {
			basicInputPV: basicInputPV,
			inputSource: corev1.PersistentVolumeSource{
				AzureDisk: &corev1.AzureDiskVolumeSource{
					DiskName:    "disk",
					DataDiskURI: "/home",
					CachingMode: toPt(corev1.AzureDataDiskCachingMode("default")),
					FSType:      toPt("az"),
				},
			},
			basicExpectedPV: basicExpectedPV,
			expectedSource: &model.PersistentVolumeSource{
				AzureDisk: &model.AzureDiskVolumeSource{
					DiskName:    "disk",
					DiskURI:     "/home",
					CachingMode: "default",
					FsType:      "az",
				},
			},
			expectedType: "AzureDisk",
		},
		"csi": {
			basicInputPV: basicInputPV,
			inputSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:       "csi",
					ReadOnly:     false,
					FSType:       "csi",
					VolumeHandle: "handle",
					VolumeAttributes: map[string]string{
						"csi":       "test",
						"namespace": "default",
					},
					NodeStageSecretRef: &corev1.SecretReference{
						Namespace: "default",
						Name:      "node_stage",
					},
				},
			},
			basicExpectedPV: basicExpectedPV,
			expectedSource: &model.PersistentVolumeSource{
				Csi: &model.CSIVolumeSource{
					Driver:       "csi",
					ReadOnly:     false,
					FsType:       "csi",
					VolumeHandle: "handle",
					VolumeAttributes: map[string]string{
						"csi":       "test",
						"namespace": "default",
					},
					NodeStageSecretRef: &model.SecretReference{
						Namespace: "default",
						Name:      "node_stage",
					},
				},
			},
			expectedType: "CSI",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.basicInputPV.Spec.PersistentVolumeSource = tc.inputSource
			tc.basicExpectedPV.Spec.PersistentVolumeType = tc.expectedType
			tc.basicExpectedPV.Spec.PersistentVolumeSource = tc.expectedSource
			tc.basicExpectedPV.Tags = append(tc.basicExpectedPV.Tags, "pv_type:"+strings.ToLower(tc.expectedType))
			assert.Equal(t, &tc.basicExpectedPV, ExtractPersistentVolume(&tc.basicInputPV))
		})
	}
}

func newInputPV() corev1.PersistentVolume {
	return corev1.PersistentVolume{
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
			ResourceVersion: "220593670",
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
}

func newExpectedPV() model.PersistentVolume {
	return model.PersistentVolume{
		Metadata: &model.Metadata{
			Annotations:       []string{"annotation:my-annotation"},
			CreationTimestamp: creationTime.Unix(),
			Labels:            []string{"app:my-app"},
			Finalizers:        []string{"foo.com/x", metav1.FinalizerOrphanDependents, "bar.com/y"},
			Name:              "pv",
			Namespace:         "project",
			ResourceVersion:   "220593670",
			Uid:               "0ff96226-578d-4679-b3c8-72e8a485c0ef",
		},
		Spec: &model.PersistentVolumeSpec{
			Capacity:             map[string]int64{string(corev1.ResourceStorage): parsedResource.Value()},
			PersistentVolumeType: "<unknown>",
			AccessModes:          []string{string(corev1.ReadWriteMany), string(corev1.ReadWriteOnce)},
			ClaimRef: &model.ObjectReference{
				Namespace: "test",
				Name:      "test-pv",
			},
			PersistentVolumeReclaimPolicy: string(corev1.PersistentVolumeReclaimRetain),
			StorageClassName:              "gold",
			MountOptions:                  []string{"ro", "soft"},
			VolumeMode:                    string(filesystem),
			NodeAffinity: []*model.NodeSelectorTerm{
				{
					MatchExpressions: []*model.LabelSelectorRequirement{
						{
							Key:      "test-key3",
							Operator: string(corev1.NodeSelectorOpIn),
							Values:   []string{"test-value1", "test-value3"},
						},
					},
					MatchFields: []*model.LabelSelectorRequirement{
						{
							Key:      "test-key2",
							Operator: string(corev1.NodeSelectorOpIn),
							Values:   []string{"test-value0", "test-value2"},
						},
					},
				},
				{
					MatchExpressions: []*model.LabelSelectorRequirement{
						{
							Key:      "test-key3",
							Operator: string(corev1.NodeSelectorOpIn),
							Values:   []string{"test-value1", "test-value3"},
						},
					},
				},
			},
		},
		Status: &model.PersistentVolumeStatus{
			Phase:   string(corev1.VolumePending),
			Message: "test",
			Reason:  "test",
		},
		Tags: []string{"pv_phase:pending"},
	}
}

func toPt[T any](s T) *T {
	return &s
}
