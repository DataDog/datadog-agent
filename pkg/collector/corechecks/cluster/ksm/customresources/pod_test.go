// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func boolPtr(b bool) *bool { return &b }

func TestContainerResourceOwnerGenerator_OwnerKind(t *testing.T) {
	memRequest := resource.MustParse("128Mi")
	container := v1.Container{
		Name: "app",
		Resources: v1.ResourceRequirements{
			Requests: v1.ResourceList{v1.ResourceMemory: memRequest},
		},
	}

	tests := []struct {
		name         string
		ownerRefs    []metav1.OwnerReference
		podLabels    map[string]string
		wantKind     string
		wantOwner    string
	}{
		{
			name: "regular deployment via replicaset",
			ownerRefs: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "myapp-6768ddc4d", Controller: boolPtr(true)},
			},
			podLabels: nil,
			wantKind:  "Deployment",
			wantOwner: "myapp",
		},
		{
			name: "argo rollout via replicaset with rollout label",
			ownerRefs: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "myapp-6768ddc4d", Controller: boolPtr(true)},
			},
			podLabels: map[string]string{kubernetes.ArgoRolloutLabelKey: "abc123"},
			wantKind:  "Rollout",
			wantOwner: "myapp",
		},
		{
			name: "replicaset with no deployment pattern",
			ownerRefs: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "standalone-rs", Controller: boolPtr(true)},
			},
			podLabels: nil,
			wantKind:  "ReplicaSet",
			wantOwner: "standalone-rs",
		},
		{
			name:      "no owner",
			ownerRefs: nil,
			podLabels: nil,
			wantKind:  "<none>",
			wantOwner: "<none>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-pod",
					Namespace:       "default",
					OwnerReferences: tt.ownerRefs,
					Labels:          tt.podLabels,
				},
				Spec: v1.PodSpec{NodeName: "node1"},
			}

			ms := containerResourceOwnerGenerator(container, pod, resourceRequests)
			require.NotEmpty(t, ms)

			// owner_kind is index 4, owner_name is index 5 in LabelValues
			for _, m := range ms {
				require.Len(t, m.LabelValues, 6)
				assert.Equal(t, tt.wantKind, m.LabelValues[4], "owner_kind mismatch")
				assert.Equal(t, tt.wantOwner, m.LabelValues[5], "owner_name mismatch")
			}
		})
	}
}
