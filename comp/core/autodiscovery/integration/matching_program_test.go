// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cel

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
)

func TestCreateMatchingProgram_EmptyRules(t *testing.T) {
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
			program, celADID, compileErr, recErr := CreateMatchingProgram(tt.rules)
			assert.Nil(t, program)
			assert.Empty(t, celADID)
			assert.NoError(t, compileErr)
			assert.NoError(t, recErr)
		})
	}
}

func TestCreateMatchingProgram_ValidRules(t *testing.T) {
	tests := []struct {
		name           string
		rules          workloadfilter.Rules
		expectedTarget workloadfilter.ResourceType
	}{
		{
			name: "single defined rule",
			rules: workloadfilter.Rules{
				Containers: []string{`container.name == "nginx" && container.image.reference == "nginx:latest"`},
			},
			expectedTarget: workloadfilter.ContainerType,
		},
		{
			name: "service rules only",
			rules: workloadfilter.Rules{
				KubeServices: []string{`kube_service.name.matches("api-service") && kube_service.namespace == "default"`},
			},
			expectedTarget: workloadfilter.KubeServiceType,
		},
		{
			name: "endpoint rules only",
			rules: workloadfilter.Rules{
				KubeEndpoints: []string{`kube_endpoint.name == "api-endpoint" && kube_endpoint.namespace == "default"`},
			},
			expectedTarget: workloadfilter.KubeEndpointType,
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
		},
		{
			name: "complex valid rules",
			rules: workloadfilter.Rules{
				KubeServices: []string{
					`kube_service.name.matches("api-.*") && kube_service.namespace == "production"`,
					`kube_service.annotations["version"] == "v2"`,
				},
			},
			expectedTarget: workloadfilter.KubeServiceType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program, celADID, compileErr, recErr := CreateMatchingProgram(tt.rules)
			assert.NotNil(t, program)
			assert.NotEmpty(t, celADID)
			assert.NoError(t, compileErr)
			assert.NoError(t, recErr)
			assert.Equal(t, tt.expectedTarget, program.GetTargetType())
		})
	}
}

func TestCreateMatchingProgram_RecommendationErrors(t *testing.T) {
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
			program, celADID, compileErr, recErr := CreateMatchingProgram(tt.rules)
			// The function should return a program but with a recommendation error
			assert.NotNil(t, program)
			assert.NotEmpty(t, celADID)
			assert.NoError(t, compileErr)
			assert.Error(t, recErr)
		})
	}
}

func TestCreateMatchingProgram_PriorityOrder(t *testing.T) {
	tests := []struct {
		name           string
		rules          workloadfilter.Rules
		expectedTarget workloadfilter.ResourceType
	}{
		{
			name: "containers have priority over services",
			rules: workloadfilter.Rules{
				Containers:   []string{`container.name == "nginx" && container.image.reference == "nginx:latest"`},
				KubeServices: []string{`kube_service.name == "api" && kube_service.namespace == "default"`},
			},
			expectedTarget: workloadfilter.ContainerType,
		},
		{
			name: "services have priority over endpoints",
			rules: workloadfilter.Rules{
				KubeServices:  []string{`kube_service.name == "api" && kube_service.namespace == "default"`},
				KubeEndpoints: []string{`kube_endpoint.name == "api-endpoint" && kube_endpoint.namespace == "default"`},
			},
			expectedTarget: workloadfilter.KubeServiceType,
		},
		{
			name: "all types present - containers win",
			rules: workloadfilter.Rules{
				Containers:    []string{`container.name == "nginx" && container.image.reference == "nginx:latest"`},
				KubeServices:  []string{`kube_service.name == "api" && kube_service.namespace == "default"`},
				KubeEndpoints: []string{`kube_endpoint.name == "api-endpoint" && kube_endpoint.namespace == "default"`},
			},
			expectedTarget: workloadfilter.ContainerType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program, celADID, compileErr, recErr := CreateMatchingProgram(tt.rules)
			assert.NotNil(t, program)
			assert.NotEmpty(t, celADID)
			assert.NoError(t, compileErr)
			assert.NoError(t, recErr)
			assert.Equal(t, tt.expectedTarget, program.GetTargetType())
		})
	}
}
