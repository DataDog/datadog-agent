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
	assert.Equal(t, "", detectCloudProvider())

	t.Run("GCP via GOOGLE_CLOUD_PROJECT", func(t *testing.T) {
		t.Setenv("GOOGLE_CLOUD_PROJECT", "my-project")
		assert.Equal(t, "GCP", detectCloudProvider())
	})

	t.Run("GCP via GCLOUD_PROJECT", func(t *testing.T) {
		t.Setenv("GCLOUD_PROJECT", "my-project")
		assert.Equal(t, "GCP", detectCloudProvider())
	})

	t.Run("GCP via GCE_METADATA_HOST", func(t *testing.T) {
		t.Setenv("GCE_METADATA_HOST", "169.254.169.254")
		assert.Equal(t, "GCP", detectCloudProvider())
	})

	t.Run("Azure via IDENTITY_ENDPOINT", func(t *testing.T) {
		t.Setenv("IDENTITY_ENDPOINT", "http://localhost")
		assert.Equal(t, "Azure", detectCloudProvider())
	})

	t.Run("Azure via MSI_ENDPOINT", func(t *testing.T) {
		t.Setenv("MSI_ENDPOINT", "http://localhost")
		assert.Equal(t, "Azure", detectCloudProvider())
	})

	t.Run("Azure via AZURE_CLIENT_ID", func(t *testing.T) {
		t.Setenv("AZURE_CLIENT_ID", "some-id")
		assert.Equal(t, "Azure", detectCloudProvider())
	})
}

func TestServiceChecksProviderCoverage(t *testing.T) {
	// Every provider in providerEnvVars must have at least one serviceCheck entry
	for provider := range providerEnvVars {
		found := false
		for _, sc := range serviceChecks {
			if sc.provider == provider {
				found = true
				break
			}
		}
		assert.True(t, found, "provider %q has env var detection but no service checks", provider)
	}
}

func TestGetCloudServiceTypeForCloudRunJob(t *testing.T) {
	t.Setenv("CLOUD_RUN_JOB", "test-job")
	cloudService := GetCloudServiceType()
	assert.Equal(t, "cloudrunjobs", cloudService.GetOrigin())

	// Verify it's the correct type
	_, ok := cloudService.(*CloudRunJobs)
	assert.True(t, ok)
}
