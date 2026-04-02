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
)

func TestInitExpectedSources(t *testing.T) {
	tests := []struct {
		name      string
		agentType wmdef.AgentType
		features  []env.Feature
		expected  map[wmdef.Kind][]wmdef.Source
	}{
		{
			name:      "not kubernetes",
			agentType: wmdef.NodeAgent,
			features:  nil, // No kubernetes
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.features != nil {
				env.SetFeatures(t, test.features...)
			}

			result := initExpectedSources(test.agentType)

			assert.Equal(t, len(test.expected), len(result))
			for kind, expectedSources := range test.expected {
				assert.ElementsMatch(t, expectedSources, result[kind])
			}
		})
	}
}
