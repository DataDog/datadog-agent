// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func TestPodWorkloadTarget(t *testing.T) {
	pod := func(labels map[string]string, owners ...workloadmeta.KubernetesPodOwner) *workloadmeta.KubernetesPod {
		return &workloadmeta.KubernetesPod{
			EntityMeta: workloadmeta.EntityMeta{Namespace: "ns", Labels: labels},
			Owners:     owners,
		}
	}

	tests := []struct {
		name     string
		pod      *workloadmeta.KubernetesPod
		expected kubernetes.WorkloadTarget
		expectOK bool
	}{
		{
			name:     "deployment through its replicaset",
			pod:      pod(nil, workloadmeta.KubernetesPodOwner{Kind: kubernetes.ReplicaSetKind, Name: "my-app-7d9f8b6c5d"}),
			expected: kubernetes.WorkloadTarget{Kind: kubernetes.DeploymentKind, Namespace: "ns", Name: "my-app"},
			expectOK: true,
		},
		{
			name: "argo rollout through its replicaset",
			pod: pod(map[string]string{kubernetes.ArgoRolloutLabelKey: "7d9f8b6c5d"},
				workloadmeta.KubernetesPodOwner{Kind: kubernetes.ReplicaSetKind, Name: "my-app-7d9f8b6c5d"}),
			expected: kubernetes.WorkloadTarget{Kind: kubernetes.RolloutKind, Namespace: "ns", Name: "my-app"},
			expectOK: true,
		},
		{
			name:     "statefulset",
			pod:      pod(nil, workloadmeta.KubernetesPodOwner{Kind: kubernetes.StatefulSetKind, Name: "db"}),
			expected: kubernetes.WorkloadTarget{Kind: kubernetes.StatefulSetKind, Namespace: "ns", Name: "db"},
			expectOK: true,
		},
		{
			name: "first resolvable owner wins",
			pod: pod(nil,
				workloadmeta.KubernetesPodOwner{Kind: "Node", Name: "worker"},
				workloadmeta.KubernetesPodOwner{Kind: kubernetes.StatefulSetKind, Name: "db"}),
			expected: kubernetes.WorkloadTarget{Kind: kubernetes.StatefulSetKind, Namespace: "ns", Name: "db"},
			expectOK: true,
		},
		{
			name:     "daemonset is not a tracked workload",
			pod:      pod(nil, workloadmeta.KubernetesPodOwner{Kind: "DaemonSet", Name: "agent"}),
			expectOK: false,
		},
		{
			name:     "bare pod",
			pod:      pod(nil),
			expectOK: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target, ok := PodWorkloadTarget(test.pod)
			assert.Equal(t, test.expectOK, ok)
			if test.expectOK {
				assert.Equal(t, test.expected, target)
			}
		})
	}
}

func TestWorkloadMetadataEntityRoundTrip(t *testing.T) {
	for _, target := range []kubernetes.WorkloadTarget{
		{Kind: kubernetes.StatefulSetKind, Namespace: "ns", Name: "db"},
		{Kind: kubernetes.RolloutKind, Namespace: "ns", Name: "web"},
	} {
		t.Run(target.Kind, func(t *testing.T) {
			entity, ok := WorkloadMetadataEntity(target)
			require.True(t, ok)
			assert.Equal(t, workloadmeta.KindKubernetesMetadata, entity.Kind)
			require.NotNil(t, entity.GVR)

			// The inverse must work from the ID alone, as for an Unset event.
			back, ok := WorkloadTargetFromMetadata(&workloadmeta.KubernetesMetadata{EntityID: entity.EntityID})
			require.True(t, ok)
			assert.Equal(t, target, back)
		})
	}

	// Same ID the generic metadata collector uses, so both merge onto one
	// entity.
	sts, _ := WorkloadMetadataEntity(kubernetes.WorkloadTarget{Kind: kubernetes.StatefulSetKind, Namespace: "ns", Name: "db"})
	assert.Equal(t, string(GenerateKubeMetadataEntityID("apps", "statefulsets", "ns", "db")), sts.ID)

	_, ok := WorkloadMetadataEntity(kubernetes.WorkloadTarget{Kind: kubernetes.DeploymentKind, Namespace: "ns", Name: "app"})
	assert.False(t, ok, "Deployments have their own entity kind")

	_, ok = WorkloadTargetFromMetadata(&workloadmeta.KubernetesMetadata{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesMetadata, ID: string(GenerateKubeMetadataEntityID("", "namespaces", "", "ns"))},
	})
	assert.False(t, ok, "a namespace is not a workload")
}
