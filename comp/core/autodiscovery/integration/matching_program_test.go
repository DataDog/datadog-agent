// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cel

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adtypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
)

func TestCreateMatchingPrograms_EmptyRules(t *testing.T) {
	tests := []struct {
		name  string
		rules workloadfilter.Rules
	}{
		{
			name:  "completely empty rules",
			rules: workloadfilter.Rules{},
		},
		{
			name: "empty rule slices",
			rules: workloadfilter.Rules{
				Containers:    []string{},
				KubeServices:  []string{},
				KubeEndpoints: []string{},
			},
		},
		{
			name: "nil rule slices",
			rules: workloadfilter.Rules{
				Containers:    nil,
				KubeServices:  nil,
				KubeEndpoints: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			programs, celADIDs, err := CreateMatchingPrograms(tt.rules, true)
			assert.Nil(t, programs)
			assert.Empty(t, celADIDs)
			assert.NoError(t, err)
		})
	}
}

func TestCreateMatchingPrograms_SingleType(t *testing.T) {
	tests := []struct {
		name           string
		rules          workloadfilter.Rules
		expectedTarget workloadfilter.ResourceType
		expectedADID   adtypes.CelIdentifier
	}{
		{
			name: "container rules only",
			rules: workloadfilter.Rules{
				Containers: []string{`container.name == "nginx" && container.image.reference == "nginx:latest"`},
			},
			expectedTarget: workloadfilter.ContainerType,
			expectedADID:   adtypes.CelContainerIdentifier,
		},
		{
			name: "process rules only",
			rules: workloadfilter.Rules{
				Processes: []string{`process.cmdline.contains("redis-server")`},
			},
			expectedTarget: workloadfilter.ProcessType,
			expectedADID:   adtypes.CelProcessIdentifier,
		},
		{
			name: "service rules only",
			rules: workloadfilter.Rules{
				KubeServices: []string{`kube_service.name.matches("api-service") && kube_service.namespace == "default"`},
			},
			expectedTarget: workloadfilter.KubeServiceType,
			expectedADID:   adtypes.CelServiceIdentifier,
		},
		{
			name: "endpoint rules only",
			rules: workloadfilter.Rules{
				KubeEndpoints: []string{`kube_endpoint.name == "api-endpoint" && kube_endpoint.namespace == "default"`},
			},
			expectedTarget: workloadfilter.KubeEndpointType,
			expectedADID:   adtypes.CelEndpointIdentifier,
		},
		{
			name: "multiple valid container rules",
			rules: workloadfilter.Rules{
				Containers: []string{
					`container.name == "nginx" && container.image.reference == "nginx:latest"`,
					`container.image.reference.matches(".*redis.*")`,
				},
			},
			expectedTarget: workloadfilter.ContainerType,
			expectedADID:   adtypes.CelContainerIdentifier,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			programs, celADIDs, err := CreateMatchingPrograms(tt.rules, true)
			require.NoError(t, err)
			require.Len(t, programs, 1)
			require.Len(t, celADIDs, 1)
			assert.Contains(t, programs, tt.expectedTarget)
			assert.Equal(t, tt.expectedTarget, programs[tt.expectedTarget].GetTargetType())
			assert.Equal(t, tt.expectedADID, celADIDs[0])
		})
	}
}

func TestCreateMatchingPrograms_MultipleTypes(t *testing.T) {
	tests := []struct {
		name            string
		rules           workloadfilter.Rules
		expectedTargets []workloadfilter.ResourceType
		expectedADIDs   []adtypes.CelIdentifier
	}{
		{
			name: "container and process rules",
			rules: workloadfilter.Rules{
				Containers: []string{`container.name == "nginx" && container.image.reference == "nginx:latest"`},
				Processes:  []string{`process.cmdline.contains("redis-server")`},
			},
			expectedTargets: []workloadfilter.ResourceType{workloadfilter.ContainerType, workloadfilter.ProcessType},
			expectedADIDs:   []adtypes.CelIdentifier{adtypes.CelContainerIdentifier, adtypes.CelProcessIdentifier},
		},
		{
			name: "container, service, and endpoint rules",
			rules: workloadfilter.Rules{
				Containers:    []string{`container.name == "nginx" && container.image.reference == "nginx:latest"`},
				KubeServices:  []string{`kube_service.name == "api" && kube_service.namespace == "default"`},
				KubeEndpoints: []string{`kube_endpoint.name == "api-endpoint" && kube_endpoint.namespace == "default"`},
			},
			expectedTargets: []workloadfilter.ResourceType{workloadfilter.ContainerType, workloadfilter.KubeServiceType, workloadfilter.KubeEndpointType},
			expectedADIDs:   []adtypes.CelIdentifier{adtypes.CelContainerIdentifier, adtypes.CelServiceIdentifier, adtypes.CelEndpointIdentifier},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			programs, celADIDs, err := CreateMatchingPrograms(tt.rules, true)
			require.NoError(t, err)
			require.Len(t, programs, len(tt.expectedTargets))
			require.Len(t, celADIDs, len(tt.expectedADIDs))
			for _, target := range tt.expectedTargets {
				assert.Contains(t, programs, target)
				assert.Equal(t, target, programs[target].GetTargetType())
			}
			for _, adID := range tt.expectedADIDs {
				assert.Contains(t, celADIDs, adID)
			}
		})
	}
}

func TestCreateMatchingPrograms_RecommendationErrors(t *testing.T) {
	tests := []struct {
		name  string
		rules workloadfilter.Rules
	}{
		{
			name: "container missing image field",
			rules: workloadfilter.Rules{
				Containers: []string{`container.name == "nginx"`},
			},
		},
		{
			name: "service missing name field",
			rules: workloadfilter.Rules{
				KubeServices: []string{`kube_service.namespace == "production"`},
			},
		},
		{
			name: "service missing namespace field",
			rules: workloadfilter.Rules{
				KubeServices: []string{`kube_service.name == "api"`},
			},
		},
		{
			name: "service missing both name and namespace fields",
			rules: workloadfilter.Rules{
				KubeServices: []string{`kube_service.annotations["version"] == "v1"`},
			},
		},
		{
			name: "endpoint missing name field",
			rules: workloadfilter.Rules{
				KubeEndpoints: []string{`kube_endpoint.namespace == "production"`},
			},
		},
		{
			name: "endpoint missing namespace field",
			rules: workloadfilter.Rules{
				KubeEndpoints: []string{`kube_endpoint.name == "api-endpoint"`},
			},
		},
		{
			name: "endpoint missing both fields",
			rules: workloadfilter.Rules{
				KubeEndpoints: []string{`kube_endpoint.annotations["monitor"] == "true"`},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			programs, celADIDs, err := CreateMatchingPrograms(tt.rules, true)
			// The function should return a program but with a recommendation error
			assert.Error(t, err)
			assert.Nil(t, programs)
			assert.Empty(t, celADIDs)
		})
	}
}

func TestCreateMatchingPrograms_RecommendationErrorFailsAll(t *testing.T) {
	// When one type has a recommendation error, the entire config fails.
	rules := workloadfilter.Rules{
		// container.name without container.image -> recommendation error
		Containers: []string{`container.name == "nginx"`},
		// valid process rule
		Processes: []string{`process.cmdline.contains("redis-server")`},
	}

	programs, celADIDs, err := CreateMatchingPrograms(rules, true)
	assert.Error(t, err)
	assert.Nil(t, programs)
	assert.Empty(t, celADIDs)
}
