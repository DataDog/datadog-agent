// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmetaimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestIsComplete(t *testing.T) {
	type completenessCase struct {
		kind         wmdef.Kind
		sources      []wmdef.Source
		wantComplete bool
	}

	tests := []struct {
		name            string
		agentType       wmdef.AgentType
		features        []env.Feature
		envVars         map[string]string
		configOverrides map[string]interface{}
		cases           []completenessCase
	}{
		{
			name:      "not kubernetes and not ECS",
			agentType: wmdef.NodeAgent,
			features:  nil, // No kubernetes and not ECS
			cases: []completenessCase{
				// No expected sources. Anything is complete.
				{
					kind:         wmdef.KindContainer,
					sources:      []wmdef.Source{wmdef.SourceRuntime},
					wantComplete: true,
				},
			},
		},
		{
			name:      "kubernetes cluster agent",
			agentType: wmdef.ClusterAgent,
			features:  []env.Feature{env.Kubernetes},
			cases: []completenessCase{
				// Not NodeAgent, so no expected sources, entities are always complete.
				{
					kind:         wmdef.KindKubernetesPod,
					sources:      []wmdef.Source{wmdef.SourceClusterOrchestrator},
					wantComplete: true,
				},
				{
					kind:         wmdef.KindKubernetesMetadata,
					sources:      []wmdef.Source{wmdef.SourceClusterOrchestrator},
					wantComplete: true,
				},
			},
		},
		{
			name:      "kubernetes node agent with runtime accessible",
			agentType: wmdef.NodeAgent,
			features:  []env.Feature{env.Kubernetes, env.Containerd},
			cases: []completenessCase{
				// Pod needs NodeOrchestrator + ClusterOrchestrator.
				{
					kind:         wmdef.KindKubernetesPod,
					sources:      []wmdef.Source{wmdef.SourceNodeOrchestrator},
					wantComplete: false,
				},
				{
					kind:         wmdef.KindKubernetesPod,
					sources:      []wmdef.Source{wmdef.SourceClusterOrchestrator},
					wantComplete: false,
				},
				{
					kind:         wmdef.KindKubernetesPod,
					sources:      []wmdef.Source{wmdef.SourceNodeOrchestrator, wmdef.SourceClusterOrchestrator},
					wantComplete: true,
				},
				// Container needs NodeOrchestrator + Runtime.
				{
					kind:         wmdef.KindContainer,
					sources:      []wmdef.Source{wmdef.SourceNodeOrchestrator},
					wantComplete: false,
				},
				{
					kind:         wmdef.KindContainer,
					sources:      []wmdef.Source{wmdef.SourceRuntime},
					wantComplete: false,
				},
				{
					kind:         wmdef.KindContainer,
					sources:      []wmdef.Source{wmdef.SourceNodeOrchestrator, wmdef.SourceRuntime},
					wantComplete: true,
				},
			},
		},
		{
			name:      "kubernetes node agent with no runtime accessible",
			agentType: wmdef.NodeAgent,
			features:  []env.Feature{env.Kubernetes},
			cases: []completenessCase{
				// Pod still needs NodeOrchestrator + ClusterOrchestrator.
				{
					kind:         wmdef.KindKubernetesPod,
					sources:      []wmdef.Source{wmdef.SourceNodeOrchestrator},
					wantComplete: false,
				},
				{
					kind:         wmdef.KindKubernetesPod,
					sources:      []wmdef.Source{wmdef.SourceNodeOrchestrator, wmdef.SourceClusterOrchestrator},
					wantComplete: true,
				},
				// Container only needs NodeOrchestrator since no runtime is accessible.
				{
					kind:         wmdef.KindContainer,
					sources:      []wmdef.Source{wmdef.SourceNodeOrchestrator},
					wantComplete: true,
				},
			},
		},
		{
			name:      "ECS EC2 on node agent with Docker runtime",
			agentType: wmdef.NodeAgent,
			features:  []env.Feature{env.ECSEC2, env.Docker},
			cases: []completenessCase{
				{
					kind:         wmdef.KindContainer,
					sources:      []wmdef.Source{wmdef.SourceNodeOrchestrator},
					wantComplete: false,
				},
				{
					kind:         wmdef.KindContainer,
					sources:      []wmdef.Source{wmdef.SourceRuntime},
					wantComplete: false,
				},
				{
					kind:         wmdef.KindContainer,
					sources:      []wmdef.Source{wmdef.SourceNodeOrchestrator, wmdef.SourceRuntime},
					wantComplete: true,
				},
			},
		},
		{
			name:      "ECS Managed on node agent with containerd runtime",
			agentType: wmdef.NodeAgent,
			features:  []env.Feature{env.ECSManagedInstances, env.Containerd},
			cases: []completenessCase{
				{
					kind:         wmdef.KindContainer,
					sources:      []wmdef.Source{wmdef.SourceNodeOrchestrator},
					wantComplete: false,
				},
				{
					kind:         wmdef.KindContainer,
					sources:      []wmdef.Source{wmdef.SourceNodeOrchestrator, wmdef.SourceRuntime},
					wantComplete: true,
				},
			},
		},
		{
			name:      "ECS Fargate has no expected sources (single collector, so always complete)",
			agentType: wmdef.NodeAgent,
			features:  []env.Feature{env.ECSFargate},
			cases: []completenessCase{
				{
					kind:         wmdef.KindContainer,
					sources:      []wmdef.Source{wmdef.SourceRuntime},
					wantComplete: true,
				},
			},
		},
		{
			name:      "ECS Managed in sidecar mode has no expected sources (single collector)",
			agentType: wmdef.NodeAgent,
			features:  []env.Feature{env.ECSManagedInstances},
			envVars: map[string]string{
				"AWS_EXECUTION_ENV": "AWS_ECS_MANAGED_INSTANCES",
			},
			configOverrides: map[string]interface{}{
				"ecs_deployment_mode": "sidecar",
			},
			cases: []completenessCase{
				{
					kind:         wmdef.KindContainer,
					sources:      []wmdef.Source{wmdef.SourceRuntime},
					wantComplete: true,
				},
			},
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
				cfg.SetWithoutSource(k, v)
			}

			tracker := newCompletenessTracker(test.agentType, cfg)

			for _, c := range test.cases {
				got := tracker.isComplete(c.kind, c.sources)
				if c.wantComplete {
					assert.Truef(t, got, "kind=%s with sources=%v should be complete", c.kind, c.sources)
				} else {
					assert.Falsef(t, got, "kind=%s with sources=%v should be incomplete", c.kind, c.sources)
				}
			}
		})
	}
}
