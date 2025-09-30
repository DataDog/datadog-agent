// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscoveryimpl

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
			program, compileErr, recErr := createMatchingProgram(tt.rules)
			assert.Nil(t, program)
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
				Containers: []string{`container.name == "nginx"`},
			},
			expectedTarget: workloadfilter.ContainerType,
		},
		{
			name: "service rules only",
			rules: workloadfilter.Rules{
				KubeServices: []string{`service.name == "api-service"`},
			},
			expectedTarget: workloadfilter.ServiceType,
		},
		{
			name: "endpoint rules only",
			rules: workloadfilter.Rules{
				KubeEndpoints: []string{`endpoint.name == "api-endpoint"`},
			},
			expectedTarget: workloadfilter.EndpointType,
		},
		{
			name: "multiple valid container rules",
			rules: workloadfilter.Rules{
				Containers: []string{
					`container.name == "nginx"`,
					`container.image.matches(".*redis.*")`,
				},
			},
			expectedTarget: workloadfilter.ContainerType,
		},
		{
			name: "complex valid rules",
			rules: workloadfilter.Rules{
				KubeServices: []string{
					`service.name.matches("api-.*") && service.namespace == "production"`,
					`service.annotations["version"] == "v2"`,
				},
			},
			expectedTarget: workloadfilter.ServiceType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program, compileErr, _ := createMatchingProgram(tt.rules)
			assert.NotNil(t, program)
			assert.NoError(t, compileErr)
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
				KubeServices: []string{`service.namespace == "production"`},
			},
		},
		{
			name: "service missing namespace field",
			rules: workloadfilter.Rules{
				KubeServices: []string{`service.name == "api"`},
			},
		},
		{
			name: "service missing both name and namespace fields",
			rules: workloadfilter.Rules{
				KubeServices: []string{`service.annotations["version"] == "v1"`},
			},
		},
		{
			name: "endpoint missing name field",
			rules: workloadfilter.Rules{
				KubeEndpoints: []string{`endpoint.namespace == "production"`},
			},
		},
		{
			name: "endpoint missing namespace field",
			rules: workloadfilter.Rules{
				KubeEndpoints: []string{`endpoint.name == "api-endpoint"`},
			},
		},
		{
			name: "endpoint missing both fields",
			rules: workloadfilter.Rules{
				KubeEndpoints: []string{`endpoint.annotations["monitor"] == "true"`},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, compileErr, recErr := createMatchingProgram(tt.rules)
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
				Containers:   []string{`container.name == "nginx"`},
				KubeServices: []string{`service.name == "api"`},
			},
			expectedTarget: workloadfilter.ContainerType,
		},
		{
			name: "services have priority over endpoints",
			rules: workloadfilter.Rules{
				KubeServices:  []string{`service.name == "api"`},
				KubeEndpoints: []string{`endpoint.name == "api-endpoint"`},
			},
			expectedTarget: workloadfilter.ServiceType,
		},
		{
			name: "all types present - containers win",
			rules: workloadfilter.Rules{
				Containers:    []string{`container.name == "nginx"`},
				KubeServices:  []string{`service.name == "api"`},
				KubeEndpoints: []string{`endpoint.name == "api-endpoint"`},
			},
			expectedTarget: workloadfilter.ContainerType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program, compileErr, _ := createMatchingProgram(tt.rules)
			assert.NotNil(t, program)
			assert.NoError(t, compileErr)
			assert.Equal(t, tt.expectedTarget, program.GetTargetType())
		})
	}
}
