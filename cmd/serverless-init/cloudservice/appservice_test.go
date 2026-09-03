// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
)

func TestGetLinuxAppServiceTags(t *testing.T) {
	service := &AppService{}

	t.Setenv("WEBSITE_SITE_NAME", "test_site_name")
	t.Setenv("REGION_NAME", "eastus")
	t.Setenv("WEBSITE_STACK", "false")

	tags := service.GetTags()
	tags["aas.environment.os"] = "linux"
	tags["aas.environment.runtime"] = "test_runtime"
	tags["aas.environment.instance_name"] = "test_instance_name"

	assert.Equal(t, map[string]string{
		"app_name":                      "test_site_name",
		"origin":                        "appservice",
		"region":                        "eastus",
		"_dd.origin":                    "appservice",
		"aas.environment.instance_id":   "unknown",
		"aas.environment.instance_name": "test_instance_name",
		"aas.environment.os":            "linux",
		"aas.environment.runtime":       "test_runtime",
		"aas.resource.group":            "",
		"aas.resource.id":               "",
		"aas.site.kind":                 "app",
		"aas.site.name":                 "test_site_name",
		"aas.site.type":                 "app",
		"aas.subscription.id":           "",
	}, tags)
}

func TestGetWindowsAppServiceTags(t *testing.T) {
	service := &AppService{}

	t.Setenv("WEBSITE_SITE_NAME", "test_site_name")
	t.Setenv("REGION_NAME", "eastus")
	t.Setenv("WEBSITE_APPSERVICEAPPLOGS_TRACE_ENABLED", "false")

	tags := service.GetTags()
	tags["aas.environment.os"] = "windows"
	tags["aas.environment.runtime"] = "test_runtime"
	tags["aas.environment.instance_name"] = "test_instance_name"

	assert.Equal(t, map[string]string{
		"app_name":                      "test_site_name",
		"origin":                        "appservice",
		"region":                        "eastus",
		"_dd.origin":                    "appservice",
		"aas.environment.instance_id":   "unknown",
		"aas.environment.instance_name": "test_instance_name",
		"aas.environment.os":            "windows",
		"aas.environment.runtime":       "test_runtime",
		"aas.resource.group":            "",
		"aas.resource.id":               "",
		"aas.site.kind":                 "app",
		"aas.site.name":                 "test_site_name",
		"aas.site.type":                 "app",
		"aas.subscription.id":           "",
	}, tags)
}

func TestAppServiceGetInventoryData(t *testing.T) {
	service := &AppService{}

	t.Setenv("WEBSITE_SITE_NAME", "test_site_name")
	t.Setenv("REGION_NAME", "eastus")
	t.Setenv("WEBSITE_OWNER_NAME", "test_subscription_id+resourcegroup-EastUSwebspace")
	t.Setenv("WEBSITE_RESOURCE_GROUP", "test_resource_group")
	t.Setenv("WEBSITE_STACK", "NODE")
	t.Setenv("WEBSITE_NODE_DEFAULT_VERSION", "~18")
	os.Unsetenv("FUNCTIONS_WORKER_RUNTIME")

	inv := service.GetInventoryData()

	assert.Equal(t, InventoryData{
		WorkloadType:        workloadTypeAzureAppService,
		ResourceID:          "/subscriptions/test_subscription_id/resourcegroups/test_resource_group/providers/microsoft.web/sites/test_site_name",
		ResourceName:        "test_site_name",
		Region:              "eastus",
		AzureSubscriptionID: "test_subscription_id",
		AzureResourceGroup:  "test_resource_group",
		Runtime:             "Node.js",
	}, inv)
}

func TestAppServiceGetInventoryDataFunctionApp(t *testing.T) {
	service := &AppService{}

	t.Setenv("WEBSITE_SITE_NAME", "test_site_name")
	t.Setenv("REGION_NAME", "eastus")
	t.Setenv("WEBSITE_OWNER_NAME", "test_subscription_id+resourcegroup-EastUSwebspace")
	t.Setenv("WEBSITE_RESOURCE_GROUP", "test_resource_group")
	t.Setenv("FUNCTIONS_WORKER_RUNTIME", "node")

	inv := service.GetInventoryData()

	assert.Equal(t, workloadTypeAzureFunction, inv.WorkloadType)
	assert.Equal(t, "/subscriptions/test_subscription_id/resourcegroups/test_resource_group/providers/microsoft.web/sites/test_site_name", inv.ResourceID)
}

func TestAppServiceGetInventoryDataWithoutAzureIDs(t *testing.T) {
	service := &AppService{}

	t.Setenv("WEBSITE_SITE_NAME", "test_site_name")
	t.Setenv("REGION_NAME", "eastus")
	os.Unsetenv("WEBSITE_OWNER_NAME")
	os.Unsetenv("WEBSITE_RESOURCE_GROUP")
	os.Unsetenv("FUNCTIONS_WORKER_RUNTIME")

	inv := service.GetInventoryData()

	assert.Equal(t, workloadTypeAzureAppService, inv.WorkloadType)
	assert.Empty(t, inv.ResourceID)
	assert.Equal(t, "test_site_name", inv.ResourceName)
	assert.Equal(t, "eastus", inv.Region)
}

func TestAppServiceShutdownEmitsMetrics(t *testing.T) {
	skipOnWindows(t)
	demux := createDemultiplexer(t)
	agent := &serverlessMetrics.ServerlessMetricAgent{Demux: demux}

	service := &AppService{}
	service.Shutdown(agent, true, nil)

	generatedMetrics, timedMetrics := demux.WaitForSamples(100 * time.Millisecond)
	assert.Empty(t, timedMetrics)
	assert.Len(t, generatedMetrics, 2)

	foundShutdown := false
	for _, sample := range generatedMetrics {
		if sample.Name == appServiceShutdownMetricName {
			foundShutdown = true
		}
	}
	assert.True(t, foundShutdown, "shutdown metric not emitted")
}

func TestAppServiceShutdownNilMetricAgent(t *testing.T) {
	service := &AppService{}
	require.NotPanics(t, func() {
		service.Shutdown(nil, true, nil)
	})
}
