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
)

func TestResolveRootOwner(t *testing.T) {
	tests := []struct {
		name     string
		owners   []workloadmeta.KubernetesPodOwner
		expected *core.FilterRootOwner
	}{
		{
			name:     "no owners",
			owners:   nil,
			expected: nil,
		},
		{
			name:     "ReplicaSet resolves to Deployment",
			owners:   []workloadmeta.KubernetesPodOwner{{Kind: "ReplicaSet", Name: "my-app-6d4f5b7c8"}},
			expected: &core.FilterRootOwner{Kind: "Deployment", Name: "my-app"},
		},
		{
			name:     "Job resolves to CronJob",
			owners:   []workloadmeta.KubernetesPodOwner{{Kind: "Job", Name: "backup-1562319360"}},
			expected: &core.FilterRootOwner{Kind: "CronJob", Name: "backup"},
		},
		{
			name:     "standalone Job stays as Job",
			owners:   []workloadmeta.KubernetesPodOwner{{Kind: "Job", Name: "one-off"}},
			expected: &core.FilterRootOwner{Kind: "Job", Name: "one-off"},
		},
		{
			name:     "Deployment is its own root",
			owners:   []workloadmeta.KubernetesPodOwner{{Kind: "Deployment", Name: "my-app"}},
			expected: &core.FilterRootOwner{Kind: "Deployment", Name: "my-app"},
		},
		{
			name:     "DaemonSet is its own root",
			owners:   []workloadmeta.KubernetesPodOwner{{Kind: "DaemonSet", Name: "fluentd"}},
			expected: &core.FilterRootOwner{Kind: "DaemonSet", Name: "fluentd"},
		},
		{
			name:     "StatefulSet is its own root",
			owners:   []workloadmeta.KubernetesPodOwner{{Kind: "StatefulSet", Name: "redis"}},
			expected: &core.FilterRootOwner{Kind: "StatefulSet", Name: "redis"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveRootOwner(tt.owners)
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
			{Kind: "ReplicaSet", Name: "my-app-6d4f5b7c8"},
		},
	}
	result := CreatePod(pod)
	assert.NotNil(t, result.FilterPod.Rootowner)
	assert.Equal(t, "Deployment", result.FilterPod.Rootowner.Kind)
	assert.Equal(t, "my-app", result.FilterPod.Rootowner.Name)
}
