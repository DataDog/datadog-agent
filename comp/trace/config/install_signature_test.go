// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInferInstallTypeFromEnvironment(t *testing.T) {
	t.Run("Non-dockerized agent", func(t *testing.T) {
		assert.Equal(t, defaultInstallType, inferInstallTypeFromEnvironment())
	})

	t.Run("Dockerized manual instrumentation", func(t *testing.T) {
		t.Setenv("DOCKER_DD_AGENT", "true")
		assert.Equal(t, defaultDockerInstallType, inferInstallTypeFromEnvironment())
	})

	t.Run("Dockerized single-step instrumentation", func(t *testing.T) {
		t.Setenv("DOCKER_DD_AGENT", "true")
		t.Setenv("DD_APM_ENABLED", "true")
		assert.Equal(t, dockerSingleStepInstallType, inferInstallTypeFromEnvironment())
	})

	t.Run("Non-standard environment variable values", func(t *testing.T) {
		t.Setenv("DOCKER_DD_AGENT", "yes")
		t.Setenv("DD_APM_ENABLED", "sure")
		assert.Equal(t, dockerSingleStepInstallType, inferInstallTypeFromEnvironment())
	})
}
