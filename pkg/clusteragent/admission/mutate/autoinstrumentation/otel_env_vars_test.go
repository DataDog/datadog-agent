// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"testing"

	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func TestOtelEquivalentOf(t *testing.T) {
	envVar := func(k, v string) corev1.EnvVar {
		return corev1.EnvVar{Name: k, Value: v}
	}

	envValueFrom := func(k, fieldPath string) corev1.EnvVar {
		return corev1.EnvVar{
			Name: k,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: fieldPath,
				},
			},
		}
	}

	container := func(env ...corev1.EnvVar) *corev1.Container {
		return &corev1.Container{Env: env}
	}

	testData := []struct {
		name        string
		ddEnvVar    string
		container   *corev1.Container
		expectCheck bool
		expectMatch bool
	}{
		{
			name:        "unknown DD env var has no OTel equivalent",
			ddEnvVar:    "DD_SOMETHING_ELSE",
			container:   container(),
			expectCheck: false,
		},
		{
			name:        "DD_SERVICE matches when OTEL_SERVICE_NAME present",
			ddEnvVar:    kubernetes.ServiceTagEnvVar,
			container:   container(envVar("OTEL_SERVICE_NAME", "my-service")),
			expectCheck: true,
			expectMatch: true,
		},
		{
			name:        "DD_SERVICE matches even when OTEL_SERVICE_NAME is ValueFrom",
			ddEnvVar:    kubernetes.ServiceTagEnvVar,
			container:   container(envValueFrom("OTEL_SERVICE_NAME", "some-field")),
			expectCheck: true,
			expectMatch: true,
		},
		{
			name:        "DD_SERVICE does not match when OTEL_SERVICE_NAME absent",
			ddEnvVar:    kubernetes.ServiceTagEnvVar,
			container:   container(envVar("OTHER", "value")),
			expectCheck: true,
			expectMatch: false,
		},
		{
			name:        "DD_VERSION matches on service.version resource attribute",
			ddEnvVar:    kubernetes.VersionTagEnvVar,
			container:   container(envVar("OTEL_RESOURCE_ATTRIBUTES", "service.version=1.2.3")),
			expectCheck: true,
			expectMatch: true,
		},
		{
			name:        "DD_VERSION does not match when key absent from resource attributes",
			ddEnvVar:    kubernetes.VersionTagEnvVar,
			container:   container(envVar("OTEL_RESOURCE_ATTRIBUTES", "deployment.environment=prod")),
			expectCheck: true,
			expectMatch: false,
		},
		{
			name:        "DD_VERSION does not match when OTEL_RESOURCE_ATTRIBUTES is ValueFrom",
			ddEnvVar:    kubernetes.VersionTagEnvVar,
			container:   container(envValueFrom("OTEL_RESOURCE_ATTRIBUTES", "some-field")),
			expectCheck: true,
			expectMatch: false,
		},
		{
			name:        "DD_ENV matches on deployment.environment resource attribute",
			ddEnvVar:    kubernetes.EnvTagEnvVar,
			container:   container(envVar("OTEL_RESOURCE_ATTRIBUTES", "deployment.environment=prod")),
			expectCheck: true,
			expectMatch: true,
		},
		{
			name:        "DD_ENV matches on deployment.environment.name resource attribute",
			ddEnvVar:    kubernetes.EnvTagEnvVar,
			container:   container(envVar("OTEL_RESOURCE_ATTRIBUTES", "deployment.environment.name=prod")),
			expectCheck: true,
			expectMatch: true,
		},
		{
			name:        "DD_ENV does not match when OTEL_RESOURCE_ATTRIBUTES absent",
			ddEnvVar:    kubernetes.EnvTagEnvVar,
			container:   container(),
			expectCheck: true,
			expectMatch: false,
		},
	}

	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
			check := otelEquivalentOf(tt.ddEnvVar)
			if !tt.expectCheck {
				require.Nil(t, check)
				return
			}

			require.NotNil(t, check)
			require.Equal(t, tt.expectMatch, check(tt.container))
		})
	}
}

func TestContainerHasEnvVarName(t *testing.T) {
	c := &corev1.Container{
		Env: []corev1.EnvVar{
			{Name: "OTEL_SERVICE_NAME", Value: "svc"},
		},
	}

	require.True(t, containerHasEnvVarName(c, "OTEL_SERVICE_NAME"))
	require.False(t, containerHasEnvVarName(c, "OTHER"))
}

func TestContainerStaticEnvVarValue(t *testing.T) {
	c := &corev1.Container{
		Env: []corev1.EnvVar{
			{Name: "STATIC", Value: "banana"},
			{
				Name: "DYNAMIC",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{FieldPath: "some-field"},
				},
			},
		},
	}

	value, ok := containerStaticEnvVarValue(c, "STATIC")
	require.True(t, ok)
	require.Equal(t, "banana", value)

	_, ok = containerStaticEnvVarValue(c, "DYNAMIC")
	require.False(t, ok)

	_, ok = containerStaticEnvVarValue(c, "MISSING")
	require.False(t, ok)
}

func TestParseOtelResourceAttributes(t *testing.T) {
	testData := []struct {
		name     string
		value    string
		expected map[string]string
	}{
		{
			name:     "empty value",
			value:    "",
			expected: map[string]string{},
		},
		{
			name:  "single pair",
			value: "service.version=1.2.3",
			expected: map[string]string{
				"service.version": "1.2.3",
			},
		},
		{
			name:  "multiple pairs with spaces",
			value: "service.version=1.2.3, deployment.environment=prod",
			expected: map[string]string{
				"service.version":        "1.2.3",
				"deployment.environment": "prod",
			},
		},
		{
			name:  "malformed pair without equals is skipped",
			value: "service.version=1.2.3,malformed,deployment.environment=prod",
			expected: map[string]string{
				"service.version":        "1.2.3",
				"deployment.environment": "prod",
			},
		},
	}

	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, parseOtelResourceAttributes(tt.value))
		})
	}
}
