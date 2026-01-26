// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package imageresolver

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

func TestIsDatadoghqRegistry(t *testing.T) {
	testCases := []struct {
		name     string
		registry string
		expected bool
	}{
		{
			name:     "gcr_io_datadoghq",
			registry: "gcr.io/datadoghq",
			expected: true,
		},
		{
			name:     "hub_docker_com_datadog",
			registry: "docker.io/datadog",
			expected: true,
		},
		{
			name:     "gallery_ecr_aws_datadog",
			registry: "public.ecr.aws/datadog",
			expected: true,
		},
		{
			name:     "docker_io_not_datadog",
			registry: "docker.io",
			expected: false,
		},
		{
			name:     "empty_registry",
			registry: "",
			expected: false,
		},
	}

	for _, tc := range testCases {

		t.Run(tc.name, func(t *testing.T) {
			mockConfig := config.NewMock(t)
			datadogRegistries := newDatadoghqRegistries(mockConfig.GetStringSlice("admission_controller.auto_instrumentation.default_dd_registries"))
			result := isDatadoghqRegistry(tc.registry, datadogRegistries)
			assert.Equal(t, tc.expected, result, "isDatadoghqRegistry(%s) should return %v", tc.registry, tc.expected)
		})
	}
}
