// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent_logs

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestIsPodOwnedByDeployment(t *testing.T) {
	tests := []struct {
		name           string
		owners         []workloadmeta.KubernetesPodOwner
		deploymentName string
		want           bool
	}{
		{
			name: "matching replicaset",
			owners: []workloadmeta.KubernetesPodOwner{
				{Kind: "ReplicaSet", Name: "my-deploy-abc123"},
			},
			deploymentName: "my-deploy",
			want:           true,
		},
		{
			name: "non-matching replicaset",
			owners: []workloadmeta.KubernetesPodOwner{
				{Kind: "ReplicaSet", Name: "other-deploy-xyz"},
			},
			deploymentName: "my-deploy",
			want:           false,
		},
		{
			name: "statefulset owner not matched",
			owners: []workloadmeta.KubernetesPodOwner{
				{Kind: "StatefulSet", Name: "my-deploy-abc123"},
			},
			deploymentName: "my-deploy",
			want:           false,
		},
		{
			name:           "no owners",
			owners:         nil,
			deploymentName: "my-deploy",
			want:           false,
		},
		{
			name: "deployment name is prefix of another deployment",
			owners: []workloadmeta.KubernetesPodOwner{
				{Kind: "ReplicaSet", Name: "my-deploy-extra-abc123"},
			},
			deploymentName: "my-deploy",
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &workloadmeta.KubernetesPod{
				Owners: tt.owners,
			}
			assert.Equal(t, tt.want, isPodOwnedByDeployment(pod, tt.deploymentName))
		})
	}
}

func TestGetContainerNames(t *testing.T) {
	pod := &workloadmeta.KubernetesPod{
		Containers: []workloadmeta.OrchestratorContainer{
			{Name: "app"},
			{Name: "sidecar"},
		},
		InitContainers: []workloadmeta.OrchestratorContainer{
			{Name: "init"},
		},
	}

	t.Run("specific container", func(t *testing.T) {
		names := getContainerNames(pod, "app")
		assert.Equal(t, []string{"app"}, names)
	})

	t.Run("all containers", func(t *testing.T) {
		names := getContainerNames(pod, "")
		require.Len(t, names, 3)
		assert.Contains(t, names, "app")
		assert.Contains(t, names, "sidecar")
		assert.Contains(t, names, "init")
	})
}

func TestReadPodLogs_MissingInputs(t *testing.T) {
	t.Run("missing namespace", func(t *testing.T) {
		inputs := podLogInputs{PodName: "my-pod"}
		err := validatePodLogInputs(inputs)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "namespace is required")
	})

	t.Run("missing both podName and deploymentName", func(t *testing.T) {
		inputs := podLogInputs{Namespace: "default"}
		err := validatePodLogInputs(inputs)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "either podName or deploymentName is required")
	})
}

// validatePodLogInputs is a helper for testing input validation logic.
func validatePodLogInputs(inputs podLogInputs) error {
	if inputs.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if inputs.PodName == "" && inputs.DeploymentName == "" {
		return fmt.Errorf("either podName or deploymentName is required")
	}
	return nil
}
