// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPodOwners(t *testing.T) {
	dsOwner := []PodOwner{
		{
			Kind: "DaemonSet",
			Name: "dd-agent-rc",
			ID:   "6a76e51c-88d7-11e7-9a0f-42010a8401cc",
		},
	}

	legacyDsAnnotation := "{\"kind\":\"SerializedReference\",\"apiVersion\":\"v1\",\"reference\":{\"kind\":\"DaemonSet\",\"namespace\":\"default\",\"name\":\"dd-agent\",\"uid\":\"12c56a58-33ca-11e6-ac8f-42010af00003\",\"apiVersion\":\"extensions\",\"resourceVersion\":\"456736\"}}\n"
	legacyDsOwner := []PodOwner{
		{
			Kind: "DaemonSet",
			Name: "dd-agent",
			ID:   "12c56a58-33ca-11e6-ac8f-42010af00003",
		},
	}

	legacyInvalidAnnotation := "{\"kind\":\"Unknown\",\"apiVersion\":\"v1\",\"reference\":{\"kind\":\"DaemonSet\",\"namespace\":\"default\",\"name\":\"dd-agent\",\"uid\":\"12c56a58-33ca-11e6-ac8f-42010af00003\",\"apiVersion\":\"extensions\",\"resourceVersion\":\"456736\"}}\n"

	for nb, tc := range []struct {
		pod            *Pod
		expectedOwners []PodOwner
	}{
		{
			pod:            &Pod{},
			expectedOwners: nil,
		},
		{
			pod: &Pod{
				Metadata: PodMetadata{
					Name:   "new-field",
					Owners: dsOwner,
				},
			},
			expectedOwners: dsOwner,
		},
		{
			pod: &Pod{
				Metadata: PodMetadata{
					Name: "legacy-pod",
					Annotations: map[string]string{
						"kubernetes.io/created-by": legacyDsAnnotation,
					},
				},
			},
			expectedOwners: legacyDsOwner,
		},
		{
			pod: &Pod{
				Metadata: PodMetadata{
					Name: "invalid-reference-kind",
					Annotations: map[string]string{
						"kubernetes.io/created-by": legacyInvalidAnnotation,
					},
				},
			},
			expectedOwners: nil,
		},
		{
			pod: &Pod{
				Metadata: PodMetadata{
					Name:   "both-keep-new",
					Owners: dsOwner,
					Annotations: map[string]string{
						"kubernetes.io/created-by": legacyDsAnnotation,
					},
				},
			},
			expectedOwners: dsOwner,
		},
	} {
		t.Run(fmt.Sprintf("case %d: %s", nb, tc.pod.Metadata.Name), func(t *testing.T) {
			assert.EqualValues(t, tc.expectedOwners, tc.pod.Owners())
		})
	}
}

func TestPodGetPersistentVolumeClaimNames(t *testing.T) {
	for nb, tc := range []struct {
		pod  *Pod
		pvcs []string
	}{
		{
			pod:  &Pod{},
			pvcs: []string{},
		},
		{
			pod: &Pod{
				Metadata: PodMetadata{
					Name: "cassandra-0",
				},
				Spec: Spec{
					Containers: []ContainerSpec{
						{
							Name:  "cassandra",
							Image: "gcr.io/google-samples/cassandra:v13",
						},
					},
				},
			},
			pvcs: []string{},
		},
		{
			pod: &Pod{
				Metadata: PodMetadata{
					Name: "cassandra-0",
				},
				Spec: Spec{
					Containers: []ContainerSpec{
						{
							Name:  "cassandra",
							Image: "gcr.io/google-samples/cassandra:v13",
						},
					},
					Volumes: []VolumeSpec{
						{
							Name: "cassandra-data",
							PersistentVolumeClaim: &PersistentVolumeClaimSpec{
								ClaimName: "cassandra-data-cassandra-0",
							},
						},
					},
				},
			},
			pvcs: []string{"cassandra-data-cassandra-0"},
		},
		{
			pod: &Pod{
				Metadata: PodMetadata{
					Name: "cassandra-0",
				},
				Spec: Spec{
					Containers: []ContainerSpec{
						{
							Name:  "cassandra",
							Image: "gcr.io/google-samples/cassandra:v13",
						},
					},
					Volumes: []VolumeSpec{
						{
							Name: "cassandra-data",
							PersistentVolumeClaim: &PersistentVolumeClaimSpec{
								ClaimName: "cassandra-data-cassandra-0",
							},
						},
						{
							Name: "another-pvc",
							PersistentVolumeClaim: &PersistentVolumeClaimSpec{
								ClaimName: "another-pvc-data-0",
							},
						},
					},
				},
			},
			pvcs: []string{"cassandra-data-cassandra-0", "another-pvc-data-0"},
		},
	} {
		t.Run(fmt.Sprintf("case %d", nb), func(t *testing.T) {
			assert.EqualValues(t, tc.pvcs, tc.pod.GetPersistentVolumeClaimNames())
		})
	}
}

func TestConditionsLastTransitionTimeUnmarshal(t *testing.T) {
	raw := `[{
		"type": "Ready",
		"status": "True",
		"reason": "ContainersReady",
		"lastTransitionTime": "2024-06-01T10:00:00Z"
	}]`

	var conditions []Conditions
	require.NoError(t, json.Unmarshal([]byte(raw), &conditions))
	require.Len(t, conditions, 1)

	expected := time.Date(2024, time.June, 1, 10, 0, 0, 0, time.UTC)
	assert.Equal(t, "Ready", conditions[0].Type)
	assert.Equal(t, "True", conditions[0].Status)
	assert.Equal(t, "ContainersReady", conditions[0].Reason)
	assert.True(t, conditions[0].LastTransitionTime.Equal(expected),
		"expected LastTransitionTime %v, got %v", expected, conditions[0].LastTransitionTime)
}
