// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestContainerAppInventoryOffReadsCurrentEnvironment(t *testing.T) {
	conf := configmock.New(t)
	conf.Set("serverless.inventory_enabled", false, model.SourceAgentRuntime)
	t.Setenv(AzureSubscriptionIdEnvVar, "Initial-Subscription")
	t.Setenv(AzureResourceGroupEnvVar, "Initial-Group")
	t.Setenv(ContainerAppNameEnvVar, "Initial-App")
	t.Setenv(ContainerAppRevision, "Initial-Revision")
	t.Setenv(ContainerAppReplicaName, "Initial-Replica")
	t.Setenv(ContainerAppDNSSuffix, "environment.eastus.azurecontainerapps.io")
	service := NewContainerApp()
	require.NoError(t, service.Init(nil))
	first := service.GetTags()
	assert.Equal(t, "/subscriptions/Initial-Subscription/resourcegroups/Initial-Group/providers/microsoft.app/containerapps/initial-app", first["resource_id"])
	assert.Equal(t, "eastus", first["region"])

	t.Setenv(AzureSubscriptionIdEnvVar, "Next-Subscription")
	t.Setenv(AzureResourceGroupEnvVar, "Next-Group")
	t.Setenv(ContainerAppNameEnvVar, "Next-App")
	t.Setenv(ContainerAppRevision, "Next-Revision")
	t.Setenv(ContainerAppReplicaName, "Next-Replica")
	t.Setenv(ContainerAppDNSSuffix, "environment.westus.azurecontainerapps.io")
	tags := service.GetTags()
	assert.Equal(t, "Next-Subscription", tags["subscription_id"])
	assert.Equal(t, "Next-Group", tags["resource_group"])
	assert.Equal(t, "Next-App", tags["app_name"])
	assert.Equal(t, "Next-Revision", tags["revision"])
	assert.Equal(t, "Next-Replica", tags["replica_name"])
	assert.Equal(t, "westus", tags["region"])
	assert.Equal(t, "/subscriptions/Next-Subscription/resourcegroups/Next-Group/providers/microsoft.app/containerapps/next-app", tags["resource_id"])
	assert.Equal(t, tags["resource_id"], tags[acaResourceID])
	assert.Equal(t, "Next-Replica", service.GetEnhancedMetricTags(tags).Usage["replica"])

	service.GetInventoryData()
	assert.Equal(t, tags, service.GetTags(), "inventory derivation must not alter telemetry tags")
	t.Setenv(AzureSubscriptionIdEnvVar, "")
	tags = service.GetTags()
	assert.NotContains(t, tags, "subscription_id")
	assert.NotContains(t, tags, acaSubscriptionID)
	assert.NotContains(t, tags, "resource_id")
	assert.NotContains(t, tags, acaResourceID)
	assert.Equal(t, "unknown", service.GetEnhancedMetricTags(tags).Base["subscription_id"])
	assert.False(t, conf.GetBool("serverless.inventory_enabled"))
}
