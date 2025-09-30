// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package program

import (
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/stretchr/testify/require"

	filterdef "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
)

func TestCELFieldConfigurationErrors(t *testing.T) {
	env, err := cel.NewEnv(
		cel.Types(&filterdef.Container{}, &filterdef.Pod{}),
		cel.Variable("container", cel.ObjectType("datadog.filter.FilterContainer")),
	)
	require.NoError(t, err)

	tests := []struct {
		name        string
		expr        string
		expectError bool
	}{
		{
			name:        "Valid container name",
			expr:        `container.name == "nginx"`,
			expectError: false,
		},
		{
			name:        "Invalid container field",
			expr:        `container.namess == "nginx"`,
			expectError: true,
		},
		{
			name:        "Valid pod name via container",
			expr:        `container.pod.name == "nginx-pod"`,
			expectError: false,
		},
		{
			name:        "Invalid pod field via container",
			expr:        `container.pod.namess == "nginx-pod"`,
			expectError: true,
		},
		{
			name:        "Valid pod annotation access",
			expr:        `container.pod.annotations["team"] == "dev"`,
			expectError: false,
		},
		{
			name:        "Invalid annotation field",
			expr:        `container.pod.annotationz["team"] == "dev"`,
			expectError: true,
		},
		{
			name:        "Completely invalid variable",
			expr:        `ctn.Name == "foo"`,
			expectError: true,
		},
		{
			name:        "Invalid method on container",
			expr:        `container.name.matchesTypo("nginx")`,
			expectError: true,
		},
		{
			name:        "Valid logical expression",
			expr:        `container.name == "nginx" || container.image == "nginx:latest"`,
			expectError: false,
		},
		{
			name:        "Multiple errors in expression",
			expr:        `container.name.other == "nginx" || container.Namesas == "nginxx"`,
			expectError: true,
		},
		{
			name:        "Valid pod namespace",
			expr:        `container.pod.namespace == "default"`,
			expectError: false,
		},
		{
			name:        "Invalid pod namespace field",
			expr:        `container.pod.namespaces == "default"`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, iss := env.Compile(tt.expr)
			errs := iss.Err()
			if tt.expectError {
				require.Error(t, errs, "expected compile error for expr: %s", tt.expr)
			} else {
				require.NoError(t, errs, "unexpected compile error for expr: %s", tt.expr)
			}
		})
	}
}
