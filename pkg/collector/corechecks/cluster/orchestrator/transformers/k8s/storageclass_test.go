// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestExtractStorageClass(t *testing.T) {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))

	t.Run("defaults", func(t *testing.T) {
		sc := &storagev1.StorageClass{
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
			},
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
				ResourceVersion: "1234",
				UID:             "c63d0b05-a340-46a6-9a38-1161ff1f9bea",
			},
			Provisioner: "provisioner",
		}

		expected := &model.StorageClass{
			AllowedTopologies: []*model.StorageClassTopology{
				{
					TopologySelectors: []*model.TopologyLabelSelector{
						{
							Key: "topology.kubernetes.io/zone",
							Values: []string{
								"us-central-1a",
								"us-central-1b",
							},
						},
					},
				},
			},
			AllowVolumeExpansion: false,
			Metadata: &model.Metadata{
				Annotations:       []string{"annotation:my-annotation"},
				CreationTimestamp: creationTime.Unix(),
				Labels:            []string{"app:my-app"},
				Name:              "storage-class",
				Namespace:         "",
				ResourceVersion:   "1234",
				Uid:               "c63d0b05-a340-46a6-9a38-1161ff1f9bea",
			},
			Provisionner:      "provisioner",
			ReclaimPolicy:     string(corev1.PersistentVolumeReclaimDelete),
			VolumeBindingMode: string(storagev1.VolumeBindingImmediate),
		}

		actual := ExtractStorageClass(sc)
		assert.Equal(t, expected, actual)
	})
	t.Run("standard", func(t *testing.T) {
		sc := &storagev1.StorageClass{
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
				ResourceVersion: "1234",
				UID:             "c63d0b05-a340-46a6-9a38-1161ff1f9bea",
			},
			Parameters: map[string]string{
				"foo": "bar",
			},
			Provisioner:       "provisioner",
			ReclaimPolicy:     pointer.Ptr(corev1.PersistentVolumeReclaimRetain),
			VolumeBindingMode: pointer.Ptr(storagev1.VolumeBindingWaitForFirstConsumer),
		}

		expected := &model.StorageClass{
			AllowedTopologies: []*model.StorageClassTopology{
				{
					TopologySelectors: []*model.TopologyLabelSelector{
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
					TopologySelectors: []*model.TopologyLabelSelector{
						{
							Key:    "topology.kubernetes.io/region",
							Values: []string{"us-central1"},
						},
					},
				},
			},
			AllowVolumeExpansion: true,
			Metadata: &model.Metadata{
				Annotations:       []string{"annotation:my-annotation"},
				CreationTimestamp: creationTime.Unix(),
				Labels:            []string{"app:my-app"},
				Name:              "storage-class",
				Namespace:         "",
				ResourceVersion:   "1234",
				Uid:               "c63d0b05-a340-46a6-9a38-1161ff1f9bea",
			},
			MountOptions:      []string{"mount-option"},
			Parameters:        map[string]string{"foo": "bar"},
			Provisionner:      "provisioner",
			ReclaimPolicy:     string(corev1.PersistentVolumeReclaimRetain),
			VolumeBindingMode: string(storagev1.VolumeBindingWaitForFirstConsumer),
		}

		actual := ExtractStorageClass(sc)
		assert.Equal(t, expected, actual)
	})
}
