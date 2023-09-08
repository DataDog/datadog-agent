// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetContainerAppTags(t *testing.T) {
	service := &ContainerApp{}

	t.Setenv("CONTAINER_APP_NAME", "test_app_name")
	t.Setenv("CONTAINER_APP_ENV_DNS_SUFFIX", "test.bluebeach.eastus.azurecontainerapps.io")
	t.Setenv("CONTAINER_APP_REVISION", "test_revision")

	t.Setenv("DD_AZURE_SUBSCRIPTION_ID", "test_subscription_id")
	t.Setenv("DD_AZURE_RESOURCE_GROUP", "test_resource_group")

	tags := service.GetTags()

	assert.Equal(t, map[string]string{
		"app_name":   "test_app_name",
		"origin":     "containerapp",
		"region":     "eastus",
		"revision":   "test_revision",
		"_dd.origin": "containerapp",
	}, tags)
}

func TestGetContainerAppTagsWithOptionalEnvVars(t *testing.T) {
	service := NewContainerApp()

	t.Setenv("CONTAINER_APP_NAME", "test_app_name")
	t.Setenv("CONTAINER_APP_ENV_DNS_SUFFIX", "test.bluebeach.eastus.azurecontainerapps.io")
	t.Setenv("CONTAINER_APP_REVISION", "test_revision")

	t.Setenv("DD_AZURE_SUBSCRIPTION_ID", "test_subscription_id")
	t.Setenv("DD_AZURE_RESOURCE_GROUP", "test_resource_group")

	err := service.Init()
	assert.NoError(t, err)

	tags := service.GetTags()

	assert.Equal(t, map[string]string{
		"app_name":        "test_app_name",
		"origin":          "containerapp",
		"region":          "eastus",
		"revision":        "test_revision",
		"_dd.origin":      "containerapp",
		"subscription_id": "test_subscription_id",
		"resource_id":     "/subscriptions/test_subscription_id/resourcegroups/test_resource_group/providers/microsoft.app/containerapps/test_app_name",
		"resource_group":  "test_resource_group",
	}, tags)

	assert.Nil(t, err)
}

func TestInitHasErrorsWhenMissingSubscriptionId(t *testing.T) {
	service := NewContainerApp()
	if os.Getenv("SERVERLESS_TEST") == "true" {
		t.Setenv("CONTAINER_APP_NAME", "test_app_name")
		t.Setenv("CONTAINER_APP_ENV_DNS_SUFFIX", "test.bluebeach.eastus.azurecontainerapps.io")
		t.Setenv("CONTAINER_APP_REVISION", "test_revision")

		t.Setenv("DD_AZURE_RESOURCE_GROUP", "test_resource_group")

		service.Init()
		return
	}

	// Re-run this test but set SERVERLESS_TEST to true to trigger the Init() function
	cmd := exec.Command(os.Args[0], "-test.run=TestInitHasErrorsWhenMissingSubscriptionId")
	cmd.Env = append(os.Environ(), "SERVERLESS_TEST=true")
	err := cmd.Run()
	if e, ok := err.(*exec.ExitError); ok && !e.Success() {
		return
	} else {
		assert.FailNow(t, "Process didn't exit when not specifying DD_AZURE_SUBSCRIPTION_ID")
	}
}

func TestInitHasErrorsWhenMissingResourceGroup(t *testing.T) {
	service := NewContainerApp()
	if os.Getenv("SERVERLESS_TEST") == "true" {
		t.Setenv("CONTAINER_APP_NAME", "test_app_name")
		t.Setenv("CONTAINER_APP_ENV_DNS_SUFFIX", "test.bluebeach.eastus.azurecontainerapps.io")
		t.Setenv("CONTAINER_APP_REVISION", "test_revision")

		t.Setenv("DD_AZURE_SUBSCRIPTION_ID", "test_subscription_id")

		service.Init()
		return
	}

	// Re-run this test but set SERVERLESS_TEST to true to trigger the Init() function
	cmd := exec.Command(os.Args[0], "-test.run=TestInitHasErrorsWhenMissingResourceGroup")
	cmd.Env = append(os.Environ(), "SERVERLESS_TEST=true")
	err := cmd.Run()
	if e, ok := err.(*exec.ExitError); ok && !e.Success() {
		return
	} else {
		assert.FailNow(t, "Process didn't exit when not specifying DD_AZURE_RESOURCE_GROUP")
	}
}
