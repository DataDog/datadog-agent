// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmetaimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestInitExpectedSources(t *testing.T) {
	tests := []struct {
		name            string
		agentType       wmdef.AgentType
		features        []env.Feature
		envVars         map[string]string
		configOverrides map[string]interface{}
		expected        map[wmdef.Kind][]wmdef.Source
	}{
		{
			name:      "not kubernetes and not ECS",
			agentType: wmdef.NodeAgent,
			features:  nil, // No kubernetes and not ECS
			expected:  map[wmdef.Kind][]wmdef.Source{},
		},
		{
			name:      "kubernetes cluster agent",
			agentType: wmdef.ClusterAgent,
			features: []env.Feature{
				env.Kubernetes,
			},
			expected: map[wmdef.Kind][]wmdef.Source{},
		},
		{
			name:      "kubernetes node agent with runtime accessible",
			agentType: wmdef.NodeAgent,
			features: []env.Feature{
				env.Kubernetes,
				env.Containerd,
			},
			expected: map[wmdef.Kind][]wmdef.Source{
				wmdef.KindKubernetesPod: {
					wmdef.SourceNodeOrchestrator,
					wmdef.SourceClusterOrchestrator,
				},
				wmdef.KindContainer: {
					wmdef.SourceRuntime,
					wmdef.SourceNodeOrchestrator,
				},
			},
		},
		{
			name:      "kubernetes node agent with no runtime accessible",
			agentType: wmdef.NodeAgent,
			features:  []env.Feature{env.Kubernetes},
			expected: map[wmdef.Kind][]wmdef.Source{
				wmdef.KindKubernetesPod: {
					wmdef.SourceNodeOrchestrator,
					wmdef.SourceClusterOrchestrator,
				},
				wmdef.KindContainer: {
					wmdef.SourceNodeOrchestrator,
				},
			},
		},
		{
			name:      "ECS EC2 on node agent with Docker runtime",
			agentType: wmdef.NodeAgent,
			features: []env.Feature{
				env.ECSEC2,
				env.Docker,
			},
			expected: map[wmdef.Kind][]wmdef.Source{
				wmdef.KindContainer: {
					wmdef.SourceNodeOrchestrator,
					wmdef.SourceRuntime,
				},
			},
		},
		{
			name:      "ECS Managed on node agent with containerd runtime",
			agentType: wmdef.NodeAgent,
			features: []env.Feature{
				env.ECSManagedInstances,
				env.Containerd,
			},
			expected: map[wmdef.Kind][]wmdef.Source{
				wmdef.KindContainer: {
					wmdef.SourceNodeOrchestrator,
					wmdef.SourceRuntime,
				},
			},
		},
		{
			name:      "ECS Fargate has no expected sources (single collector, so always complete)",
			agentType: wmdef.NodeAgent,
			features:  []env.Feature{env.ECSFargate},
			expected:  map[wmdef.Kind][]wmdef.Source{},
		},
		{
			name:      "ECS Managed in sidecar mode has no expected sources (single collector)",
			agentType: wmdef.NodeAgent,
			features: []env.Feature{
				env.ECSManagedInstances,
			},
			envVars: map[string]string{
				"AWS_EXECUTION_ENV": "AWS_ECS_MANAGED_INSTANCES",
			},
			configOverrides: map[string]interface{}{
				"ecs_deployment_mode": "sidecar",
			},
			expected: map[wmdef.Kind][]wmdef.Source{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.features != nil {
				env.SetFeatures(t, test.features...)
			}

			for k, v := range test.envVars {
				t.Setenv(k, v)
			}

			cfg := configmock.New(t)
			for k, v := range test.configOverrides {
				cfg.SetInTest(k, v)
			}

			result := initExpectedSources(test.agentType, cfg)

			require.Equal(t, len(test.expected), len(result))
			for kind, expectedSources := range test.expected {
				assert.ElementsMatch(t, expectedSources, result[kind])
			}
		})
	}
}
