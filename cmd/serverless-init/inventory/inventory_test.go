// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package inventory

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	"github.com/stretchr/testify/assert"
)

func TestCloudProviderFromOrigin(t *testing.T) {
	tests := []struct {
		origin string
		want   string
	}{
		{cloudservice.CloudRunOrigin, "gcp"},
		{cloudservice.CloudRunJobsOrigin, "gcp"},
		{cloudservice.ContainerAppOrigin, "azure"},
		{cloudservice.AppServiceOrigin, "azure"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, cloudProviderFromOrigin(tt.origin), "origin=%s", tt.origin)
	}
}

func TestWorkloadTypeFromOrigin(t *testing.T) {
	t.Run("cloud_run_service", func(t *testing.T) {
		assert.Equal(t, "cloud_run_service", workloadTypeFromOrigin(cloudservice.CloudRunOrigin))
	})
	t.Run("cloud_run_function", func(t *testing.T) {
		t.Setenv("FUNCTION_TARGET", "myHandler")
		assert.Equal(t, "cloud_run_function", workloadTypeFromOrigin(cloudservice.CloudRunOrigin))
	})
	t.Run("cloud_run_job", func(t *testing.T) {
		assert.Equal(t, "cloud_run_job", workloadTypeFromOrigin(cloudservice.CloudRunJobsOrigin))
	})
	t.Run("container_app", func(t *testing.T) {
		assert.Equal(t, "container_app", workloadTypeFromOrigin(cloudservice.ContainerAppOrigin))
	})
	t.Run("app_service", func(t *testing.T) {
		assert.Equal(t, "app_service", workloadTypeFromOrigin(cloudservice.AppServiceOrigin))
	})
}

func TestResourceNameFromOrigin(t *testing.T) {
	t.Run("cloud_run", func(t *testing.T) {
		t.Setenv(cloudservice.ServiceNameEnvVar, "my-service")
		assert.Equal(t, "my-service", resourceNameFromOrigin(cloudservice.CloudRunOrigin))
	})
	t.Run("container_app", func(t *testing.T) {
		t.Setenv(cloudservice.ContainerAppNameEnvVar, "my-app")
		assert.Equal(t, "my-app", resourceNameFromOrigin(cloudservice.ContainerAppOrigin))
	})
	t.Run("app_service", func(t *testing.T) {
		t.Setenv(cloudservice.WebsiteName, "my-webapp")
		assert.Equal(t, "my-webapp", resourceNameFromOrigin(cloudservice.AppServiceOrigin))
	})
}

func TestDeploymentModelFromConf(t *testing.T) {
	assert.Equal(t, "sidecar", deploymentModelFromConf(mode.Conf{SidecarMode: true}))
	assert.Equal(t, "init-container", deploymentModelFromConf(mode.Conf{SidecarMode: false}))
}

func TestDetectRuntime(t *testing.T) {
	t.Run("python", func(t *testing.T) {
		t.Setenv("PYTHON_VERSION", "3.11.9")
		assert.Equal(t, "python3.11.9", detectRuntime())
	})
	t.Run("node", func(t *testing.T) {
		t.Setenv("NODE_VERSION", "20.11.0")
		assert.Equal(t, "node20.11.0", detectRuntime())
	})
	t.Run("unknown", func(t *testing.T) {
		os.Unsetenv("PYTHON_VERSION")
		os.Unsetenv("NODE_VERSION")
		os.Unsetenv("JAVA_VERSION")
		os.Unsetenv("DOTNET_VERSION")
		os.Unsetenv("RUBY_VERSION")
		assert.Equal(t, "", detectRuntime())
	})
}

func TestFirstEnv(t *testing.T) {
	t.Setenv("KEY_A", "val_a")
	assert.Equal(t, "val_a", firstEnv("KEY_A", "KEY_B"))
	assert.Equal(t, "val_a", firstEnv("KEY_MISSING", "KEY_A"))
	assert.Equal(t, "", firstEnv("KEY_MISSING1", "KEY_MISSING2"))
}
