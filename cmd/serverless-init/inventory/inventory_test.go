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
		assert.Equal(t, "cloud_run_service", workloadTypeFromOrigin(cloudservice.CloudRunOrigin, map[string]string{}))
	})
	t.Run("cloud_function_gen2_via_env", func(t *testing.T) {
		t.Setenv("FUNCTION_TARGET", "myHandler")
		assert.Equal(t, "cloud_function_gen2", workloadTypeFromOrigin(cloudservice.CloudRunOrigin, map[string]string{}))
	})
	t.Run("cloud_function_gen2_via_tag", func(t *testing.T) {
		assert.Equal(t, "cloud_function_gen2", workloadTypeFromOrigin(cloudservice.CloudRunOrigin, map[string]string{"build_function_target": "myHandler"}))
	})
	t.Run("cloud_run_job", func(t *testing.T) {
		assert.Equal(t, "cloud_run_job", workloadTypeFromOrigin(cloudservice.CloudRunJobsOrigin, map[string]string{}))
	})
	t.Run("azure_container_app", func(t *testing.T) {
		assert.Equal(t, "azure_container_app", workloadTypeFromOrigin(cloudservice.ContainerAppOrigin, map[string]string{}))
	})
	t.Run("azure_app_service", func(t *testing.T) {
		assert.Equal(t, "azure_app_service", workloadTypeFromOrigin(cloudservice.AppServiceOrigin, map[string]string{}))
	})
}

func TestRegionFromOriginAndTags(t *testing.T) {
	t.Run("cloud_run", func(t *testing.T) {
		tags := map[string]string{"location": "us-central1"}
		assert.Equal(t, "us-central1", regionFromOriginAndTags(cloudservice.CloudRunOrigin, tags))
	})
	t.Run("cloud_run_job", func(t *testing.T) {
		tags := map[string]string{"location": "europe-west1"}
		assert.Equal(t, "europe-west1", regionFromOriginAndTags(cloudservice.CloudRunJobsOrigin, tags))
	})
	t.Run("container_app", func(t *testing.T) {
		tags := map[string]string{"region": "eastus"}
		assert.Equal(t, "eastus", regionFromOriginAndTags(cloudservice.ContainerAppOrigin, tags))
	})
	t.Run("app_service", func(t *testing.T) {
		tags := map[string]string{"region": "westeurope"}
		assert.Equal(t, "westeurope", regionFromOriginAndTags(cloudservice.AppServiceOrigin, tags))
	})
	t.Run("missing", func(t *testing.T) {
		assert.Equal(t, "", regionFromOriginAndTags(cloudservice.CloudRunOrigin, map[string]string{}))
	})
}

func TestResourceNameFromOrigin(t *testing.T) {
	t.Run("cloud_run_service", func(t *testing.T) {
		tags := map[string]string{"gcr.resource_name": "projects/p/locations/l/services/my-service"}
		assert.Equal(t, "my-service", resourceNameFromOrigin(cloudservice.CloudRunOrigin, tags))
	})
	t.Run("cloud_run_function_via_env", func(t *testing.T) {
		t.Setenv("FUNCTION_TARGET", "myHandler")
		tags := map[string]string{"gcrfx.resource_name": "projects/p/locations/l/services/s/functions/myHandler"}
		assert.Equal(t, "myHandler", resourceNameFromOrigin(cloudservice.CloudRunOrigin, tags))
	})
	t.Run("cloud_run_function_via_tag", func(t *testing.T) {
		tags := map[string]string{
			"gcr.resource_name":     "projects/p/locations/l/services/my-fn-svc",
			"gcrfx.resource_name":   "projects/p/locations/l/services/my-fn-svc/functions/myHandler",
			"build_function_target": "myHandler",
		}
		assert.Equal(t, "myHandler", resourceNameFromOrigin(cloudservice.CloudRunOrigin, tags))
	})
	t.Run("container_app", func(t *testing.T) {
		t.Setenv(cloudservice.ContainerAppNameEnvVar, "my-app")
		assert.Equal(t, "my-app", resourceNameFromOrigin(cloudservice.ContainerAppOrigin, nil))
	})
	t.Run("app_service", func(t *testing.T) {
		t.Setenv(cloudservice.WebsiteName, "my-webapp")
		assert.Equal(t, "my-webapp", resourceNameFromOrigin(cloudservice.AppServiceOrigin, nil))
	})
}

func TestResourceIDFromTags(t *testing.T) {
	t.Run("cloud_run_service", func(t *testing.T) {
		tags := map[string]string{"gcr.resource_name": "projects/p/locations/l/services/my-service"}
		assert.Equal(t, "//run.googleapis.com/projects/p/locations/l/services/my-service", resourceIDFromTags(cloudservice.CloudRunOrigin, tags))
	})
	t.Run("cloud_run_function", func(t *testing.T) {
		t.Setenv("FUNCTION_TARGET", "myHandler")
		tags := map[string]string{"gcrfx.resource_name": "projects/p/locations/l/services/s/functions/myHandler"}
		assert.Equal(t, "//run.googleapis.com/projects/p/locations/l/services/s/functions/myHandler", resourceIDFromTags(cloudservice.CloudRunOrigin, tags))
	})
	t.Run("cloud_run_job", func(t *testing.T) {
		tags := map[string]string{"gcrj.resource_name": "projects/p/locations/l/jobs/my-job"}
		assert.Equal(t, "//run.googleapis.com/projects/p/locations/l/jobs/my-job", resourceIDFromTags(cloudservice.CloudRunJobsOrigin, tags))
	})
	t.Run("container_app", func(t *testing.T) {
		tags := map[string]string{"resource_id": "/subscriptions/sub-1/resourcegroups/rg-1/providers/microsoft.app/containerapps/My-App"}
		assert.Equal(t, "//microsoft.azure/containerApps/sub-1/rg-1/my-app", resourceIDFromTags(cloudservice.ContainerAppOrigin, tags))
	})
	t.Run("app_service_full", func(t *testing.T) {
		t.Setenv(cloudservice.AzureSubscriptionIdEnvVar, "sub-1")
		t.Setenv(cloudservice.AzureResourceGroupEnvVar, "rg-1")
		t.Setenv(cloudservice.WebsiteName, "My-Webapp")
		assert.Equal(t, "//microsoft.azure/appServices/sub-1/rg-1/my-webapp", resourceIDFromTags(cloudservice.AppServiceOrigin, nil))
	})
	t.Run("app_service_missing", func(t *testing.T) {
		os.Unsetenv(cloudservice.AzureSubscriptionIdEnvVar)
		os.Unsetenv(cloudservice.AzureResourceGroupEnvVar)
		os.Unsetenv(cloudservice.WebsiteName)
		assert.Equal(t, "", resourceIDFromTags(cloudservice.AppServiceOrigin, nil))
	})
	t.Run("empty_path", func(t *testing.T) {
		assert.Equal(t, "", resourceIDFromTags(cloudservice.CloudRunOrigin, map[string]string{}))
	})
}

func TestDeploymentModelFromConf(t *testing.T) {
	assert.Equal(t, "sidecar", deploymentModelFromConf(mode.Conf{SidecarMode: true}))
	assert.Equal(t, "in-container", deploymentModelFromConf(mode.Conf{SidecarMode: false}))
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
