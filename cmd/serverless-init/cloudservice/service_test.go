// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCloudServiceType(t *testing.T) {
	// Mock getenv to isolate from CI runner environment (e.g. GCP)
	originalGetenv := getenv
	getenv = func(string) string { return "" }
	t.Cleanup(func() { getenv = originalGetenv })

	assert.Equal(t, "local", GetCloudServiceType().GetOrigin())

	t.Setenv(ContainerAppNameEnvVar, "test-name")
	assert.Equal(t, "containerapp", GetCloudServiceType().GetOrigin())

	t.Setenv(ServiceNameEnvVar, "test-name")
	assert.Equal(t, "cloudrun", GetCloudServiceType().GetOrigin())

	os.Unsetenv(ContainerAppNameEnvVar)
	os.Unsetenv(ServiceNameEnvVar)
	t.Setenv(WebsiteStack, "false")
	assert.Equal(t, "appservice", GetCloudServiceType().GetOrigin())
}

func TestDetectCloudProvider(t *testing.T) {
	originalGetenv := getenv
	t.Cleanup(func() { getenv = originalGetenv })

	getenv = func(string) string { return "" }
	assert.Equal(t, "", detectCloudProvider())

	t.Run("GCP via GOOGLE_CLOUD_PROJECT", func(t *testing.T) {
		getenv = func(key string) string {
			if key == "GOOGLE_CLOUD_PROJECT" {
				return "my-project"
			}
			return ""
		}
		assert.Equal(t, "GCP", detectCloudProvider())
	})

	t.Run("GCP via GCLOUD_PROJECT", func(t *testing.T) {
		getenv = func(key string) string {
			if key == "GCLOUD_PROJECT" {
				return "my-project"
			}
			return ""
		}
		assert.Equal(t, "GCP", detectCloudProvider())
	})

	t.Run("GCP via GCE_METADATA_HOST", func(t *testing.T) {
		getenv = func(key string) string {
			if key == "GCE_METADATA_HOST" {
				return "169.254.169.254"
			}
			return ""
		}
		assert.Equal(t, "GCP", detectCloudProvider())
	})

	t.Run("Azure via IDENTITY_ENDPOINT", func(t *testing.T) {
		getenv = func(key string) string {
			if key == "IDENTITY_ENDPOINT" {
				return "http://localhost"
			}
			return ""
		}
		assert.Equal(t, "Azure", detectCloudProvider())
	})

	t.Run("Azure via MSI_ENDPOINT", func(t *testing.T) {
		getenv = func(key string) string {
			if key == "MSI_ENDPOINT" {
				return "http://localhost"
			}
			return ""
		}
		assert.Equal(t, "Azure", detectCloudProvider())
	})

	t.Run("Azure via AZURE_CLIENT_ID", func(t *testing.T) {
		getenv = func(key string) string {
			if key == "AZURE_CLIENT_ID" {
				return "some-id"
			}
			return ""
		}
		assert.Equal(t, "Azure", detectCloudProvider())
	})
}

func TestGetCloudServiceTypeForCloudRunJob(t *testing.T) {
	t.Setenv("CLOUD_RUN_JOB", "test-job")
	cloudService := GetCloudServiceType()
	assert.Equal(t, "cloudrunjobs", cloudService.GetOrigin())

	// Verify it's the correct type
	_, ok := cloudService.(*CloudRunJobs)
	assert.True(t, ok)
}
