// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"testing"

	"github.com/stretchr/testify/assert"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func boolPtr(b bool) *bool { return &b }

func TestResolveRootOwner(t *testing.T) {
	tests := []struct {
		name      string
		owners    []workloadmeta.KubernetesPodOwner
		podLabels map[string]string
		expected  *core.FilterRootOwner
	}{
		{
			name:     "no owners",
			owners:   nil,
			expected: nil,
		},
		{
			name:     "ReplicaSet resolves to Deployment",
			owners:   []workloadmeta.KubernetesPodOwner{{Kind: "ReplicaSet", Name: "my-app-6d4f5b7c8", Controller: boolPtr(true)}},
			expected: &core.FilterRootOwner{Kind: "Deployment", Name: "my-app"},
		},
		{
			name:      "ReplicaSet with Argo rollout label resolves to Rollout",
			owners:    []workloadmeta.KubernetesPodOwner{{Kind: "ReplicaSet", Name: "my-rollout-9b8dc4bd6", Controller: boolPtr(true)}},
			podLabels: map[string]string{kubernetes.ArgoRolloutLabelKey: "9b8dc4bd6"},
			expected:  &core.FilterRootOwner{Kind: "Rollout", Name: "my-rollout"},
		},
		{
			name:      "ReplicaSet with Argo rollout label but no hash suffix stays as ReplicaSet",
			owners:    []workloadmeta.KubernetesPodOwner{{Kind: "ReplicaSet", Name: "invalid-name", Controller: boolPtr(true)}},
			podLabels: map[string]string{kubernetes.ArgoRolloutLabelKey: "invalid"},
			expected:  &core.FilterRootOwner{Kind: "ReplicaSet", Name: "invalid-name"},
		},
		{
			name:      "ReplicaSet with empty Argo rollout label resolves to Deployment",
			owners:    []workloadmeta.KubernetesPodOwner{{Kind: "ReplicaSet", Name: "my-app-6d4f5b7c8", Controller: boolPtr(true)}},
			podLabels: map[string]string{kubernetes.ArgoRolloutLabelKey: ""},
			expected:  &core.FilterRootOwner{Kind: "Deployment", Name: "my-app"},
		},
		{
			name:     "Job resolves to CronJob",
			owners:   []workloadmeta.KubernetesPodOwner{{Kind: "Job", Name: "backup-1562319360", Controller: boolPtr(true)}},
			expected: &core.FilterRootOwner{Kind: "CronJob", Name: "backup"},
		},
		{
			name:     "standalone Job stays as Job",
			owners:   []workloadmeta.KubernetesPodOwner{{Kind: "Job", Name: "one-off", Controller: boolPtr(true)}},
			expected: &core.FilterRootOwner{Kind: "Job", Name: "one-off"},
		},
		{
			name:     "Deployment is its own root",
			owners:   []workloadmeta.KubernetesPodOwner{{Kind: "Deployment", Name: "my-app", Controller: boolPtr(true)}},
			expected: &core.FilterRootOwner{Kind: "Deployment", Name: "my-app"},
		},
		{
			name:     "DaemonSet is its own root",
			owners:   []workloadmeta.KubernetesPodOwner{{Kind: "DaemonSet", Name: "fluentd", Controller: boolPtr(true)}},
			expected: &core.FilterRootOwner{Kind: "DaemonSet", Name: "fluentd"},
		},
		{
			name:     "StatefulSet is its own root",
			owners:   []workloadmeta.KubernetesPodOwner{{Kind: "StatefulSet", Name: "redis", Controller: boolPtr(true)}},
			expected: &core.FilterRootOwner{Kind: "StatefulSet", Name: "redis"},
		},
		{
			name:     "StrimziPodSet is its own root",
			owners:   []workloadmeta.KubernetesPodOwner{{Kind: "StrimziPodSet", Name: "my-cluster-kafka", Controller: boolPtr(true)}},
			expected: &core.FilterRootOwner{Kind: "StrimziPodSet", Name: "my-cluster-kafka"},
		},
		{
			name: "prefers controller owner over non-controller",
			owners: []workloadmeta.KubernetesPodOwner{
				{Kind: "Node", Name: "node-1", Controller: boolPtr(false)},
				{Kind: "ReplicaSet", Name: "my-app-6d4f5b7c8", Controller: boolPtr(true)},
			},
			expected: &core.FilterRootOwner{Kind: "Deployment", Name: "my-app"},
		},
		{
			name: "no controller marked falls back to first owner",
			owners: []workloadmeta.KubernetesPodOwner{
				{Kind: "Node", Name: "node-1", Controller: boolPtr(false)},
				{Kind: "ReplicaSet", Name: "my-app-6d4f5b7c8", Controller: boolPtr(false)},
			},
			expected: &core.FilterRootOwner{Kind: "Node", Name: "node-1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveRootOwner(tt.owners, tt.podLabels)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCreatePodWithRootOwner(t *testing.T) {
	pod := &workloadmeta.KubernetesPod{
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "my-app-6d4f5b7c8-abc12",
			Namespace: "default",
		},
		Owners: []workloadmeta.KubernetesPodOwner{
			{Kind: "ReplicaSet", Name: "my-app-6d4f5b7c8", Controller: boolPtr(true)},
		},
	}
	result := CreatePod(pod)
	assert.NotNil(t, result.FilterPod.Rootowner)
	assert.Equal(t, "Deployment", result.FilterPod.Rootowner.Kind)
	assert.Equal(t, "my-app", result.FilterPod.Rootowner.Name)
}

func TestCreatePodWithRolloutRootOwner(t *testing.T) {
	pod := &workloadmeta.KubernetesPod{
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "my-rollout-9b8dc4bd6-abc12",
			Namespace: "default",
			Labels:    map[string]string{kubernetes.ArgoRolloutLabelKey: "9b8dc4bd6"},
		},
		Owners: []workloadmeta.KubernetesPodOwner{
			{Kind: "ReplicaSet", Name: "my-rollout-9b8dc4bd6", Controller: boolPtr(true)},
		},
	}
	result := CreatePod(pod)
	assert.NotNil(t, result.FilterPod.Rootowner)
	assert.Equal(t, "Rollout", result.FilterPod.Rootowner.Kind)
	assert.Equal(t, "my-rollout", result.FilterPod.Rootowner.Name)
}

func TestCreatePodWithStrimziPodSetRootOwner(t *testing.T) {
	pod := &workloadmeta.KubernetesPod{
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "my-cluster-kafka-0",
			Namespace: "default",
		},
		Owners: []workloadmeta.KubernetesPodOwner{
			{Kind: "StrimziPodSet", Name: "my-cluster-kafka", Controller: boolPtr(true)},
		},
	}
	result := CreatePod(pod)
	assert.NotNil(t, result.FilterPod.Rootowner)
	assert.Equal(t, "StrimziPodSet", result.FilterPod.Rootowner.Kind)
	assert.Equal(t, "my-cluster-kafka", result.FilterPod.Rootowner.Name)
}
