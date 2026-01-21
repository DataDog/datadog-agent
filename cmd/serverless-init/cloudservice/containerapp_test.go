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
	service := NewContainerApp()

	t.Setenv("CONTAINER_APP_NAME", "test_app_name")
	t.Setenv("CONTAINER_APP_ENV_DNS_SUFFIX", "test.bluebeach.eastus.azurecontainerapps.io")
	t.Setenv("CONTAINER_APP_REVISION", "test_revision")
	t.Setenv("CONTAINER_APP_REPLICA_NAME", "test--6nyz8z7-b845f7667-m7hlv")

	t.Setenv("DD_AZURE_SUBSCRIPTION_ID", "test_subscription_id")
	t.Setenv("DD_AZURE_RESOURCE_GROUP", "test_resource_group")

	err := service.Init(nil)
	assert.NoError(t, err)

	tags := service.GetTags()

	assert.Equal(t, map[string]string{
		"app_name":            "test_app_name",
		"origin":              "containerapp",
		"region":              "eastus",
		"revision":            "test_revision",
		"replica_name":        "test--6nyz8z7-b845f7667-m7hlv",
		"_dd.origin":          "containerapp",
		"subscription_id":     "test_subscription_id",
		"resource_id":         "/subscriptions/test_subscription_id/resourcegroups/test_resource_group/providers/microsoft.app/containerapps/test_app_name",
		"resource_group":      "test_resource_group",
		"aca.app.name":        "test_app_name",
		"aca.app.region":      "eastus",
		"aca.app.revision":    "test_revision",
		"aca.replica.name":    "test--6nyz8z7-b845f7667-m7hlv",
		"aca.resource.id":     "/subscriptions/test_subscription_id/resourcegroups/test_resource_group/providers/microsoft.app/containerapps/test_app_name",
		"aca.resource.group":  "test_resource_group",
		"aca.subscription.id": "test_subscription_id",
	}, tags)

	assert.Nil(t, err)
}

func TestGetContainerAppTagsBeforeInit(t *testing.T) {
	// This test demonstrates that GetTags can be called before Init
	// and will correctly fall back to environment variables for subscription_id and resource_group
	service := NewContainerApp()
	t.Setenv("CONTAINER_APP_NAME", "test_app")
	t.Setenv("CONTAINER_APP_ENV_DNS_SUFFIX", "test.bluebeach.westus.azurecontainerapps.io")
	t.Setenv("CONTAINER_APP_REVISION", "test_revision")
	t.Setenv("CONTAINER_APP_REPLICA_NAME", "test--replica")

	t.Setenv("DD_AZURE_SUBSCRIPTION_ID", "test_subscription_id")
	t.Setenv("DD_AZURE_RESOURCE_GROUP", "test_resource_group")

	// Call GetTags BEFORE Init - it should still get the values from env vars
	tags := service.GetTags()

	err := service.Init(nil)
	assert.NoError(t, err)

	// Verify that subscription_id and resource_group are populated from env vars
	assert.Equal(t, "test_subscription_id", tags["subscription_id"])
	assert.Equal(t, "test_resource_group", tags["resource_group"])
	assert.Equal(t, "test_subscription_id", tags["aca.subscription.id"])
	assert.Equal(t, "test_resource_group", tags["aca.resource.group"])
	assert.Equal(t, "/subscriptions/test_subscription_id/resourcegroups/test_resource_group/providers/microsoft.app/containerapps/test_app", tags["resource_id"])
	assert.Equal(t, "/subscriptions/test_subscription_id/resourcegroups/test_resource_group/providers/microsoft.app/containerapps/test_app", tags["aca.resource.id"])
}

func TestInitHasErrorsWhenMissingSubscriptionId(t *testing.T) {
	service := NewContainerApp()
	if os.Getenv("SERVERLESS_TEST") == "true" {
		t.Setenv("CONTAINER_APP_NAME", "test_app_name")
		t.Setenv("CONTAINER_APP_ENV_DNS_SUFFIX", "test.bluebeach.eastus.azurecontainerapps.io")
		t.Setenv("CONTAINER_APP_REVISION", "test_revision")
		t.Setenv("CONTAINER_APP_REPLICA_NAME", "test--6nyz8z7-b845f7667-m7hlv")

		t.Setenv("DD_AZURE_RESOURCE_GROUP", "test_resource_group")

		service.Init(nil)
		return
	}

	// Re-run this test but set SERVERLESS_TEST to true to trigger the Init() function
	cmd := exec.Command(os.Args[0], "-test.run=TestInitHasErrorsWhenMissingSubscriptionId")
	cmd.Env = append(os.Environ(), "SERVERLESS_TEST=true")
	err := cmd.Run()
	if e, ok := err.(*exec.ExitError); ok && !e.Success() {
		return
	} else { //nolint:revive // TODO(SERV) Fix revive linter
		assert.FailNow(t, "Process didn't exit when not specifying DD_AZURE_SUBSCRIPTION_ID")
	}
}

func TestInitHasErrorsWhenMissingResourceGroup(t *testing.T) {
	service := NewContainerApp()
	if os.Getenv("SERVERLESS_TEST") == "true" {
		t.Setenv("CONTAINER_APP_NAME", "test_app_name")
		t.Setenv("CONTAINER_APP_ENV_DNS_SUFFIX", "test.bluebeach.eastus.azurecontainerapps.io")
		t.Setenv("CONTAINER_APP_REVISION", "test_revision")
		t.Setenv("CONTAINER_APP_REPLICA_NAME", "test--6nyz8z7-b845f7667-m7hlv")

		t.Setenv("DD_AZURE_SUBSCRIPTION_ID", "test_subscription_id")

		service.Init(nil)
		return
	}

	// Re-run this test but set SERVERLESS_TEST to true to trigger the Init() function
	cmd := exec.Command(os.Args[0], "-test.run=TestInitHasErrorsWhenMissingResourceGroup")
	cmd.Env = append(os.Environ(), "SERVERLESS_TEST=true")
	err := cmd.Run()
	if e, ok := err.(*exec.ExitError); ok && !e.Success() {
		return
	} else { //nolint:revive // TODO(SERV) Fix revive linter
		assert.FailNow(t, "Process didn't exit when not specifying DD_AZURE_RESOURCE_GROUP")
	}
}
