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
	t.Setenv("FUNCTIONS_WORKER_RUNTIME", "")
	require.NoError(t, os.Unsetenv("FUNCTIONS_WORKER_RUNTIME"))

	inv := service.GetInventoryData()

	assert.Equal(t, InventoryData{
		WorkloadType:        workloadTypeAzureAppService,
		ResourceID:          "/subscriptions/test_subscription_id/resourcegroups/test_resource_group/providers/microsoft.web/sites/test_site_name",
		ResourceName:        "test_site_name",
		Region:              "eastus",
		AzureSubscriptionID: "test_subscription_id",
		AzureResourceGroup:  "test_resource_group",
		RuntimeCandidates:   []string{"", "Node.js"},
	}, inv)
	assert.True(t, service.CanCollectInventory())
}

func TestAppServiceGetInventoryDataFunctionApp(t *testing.T) {
	service := &AppService{}

	t.Setenv("WEBSITE_SITE_NAME", "test_site_name")
	t.Setenv("REGION_NAME", "eastus")
	t.Setenv("WEBSITE_OWNER_NAME", "test_subscription_id+resourcegroup-EastUSwebspace")
	t.Setenv("WEBSITE_RESOURCE_GROUP", "test_resource_group")
	t.Setenv("FUNCTIONS_WORKER_RUNTIME", "node")

	inv := service.GetInventoryData()

	assert.True(t, service.CanCollectInventory())
	assert.Equal(t, workloadTypeAzureFunction, inv.WorkloadType)
	assert.Equal(t, "/subscriptions/test_subscription_id/resourcegroups/test_resource_group/providers/microsoft.web/sites/test_site_name", inv.ResourceID)
}

func TestAppServiceInventoryRuntimeCandidates(t *testing.T) {
	for _, tt := range []struct {
		name   string
		stack  string
		worker string
		want   string
	}{
		{name: "node", stack: "NODE", want: "Node.js"},
		{name: "python", stack: "PYTHON", want: "Python"},
		{name: "java", stack: "JAVA", want: "Java"},
		{name: "tomcat", stack: "TOMCAT", want: "Java"},
		{name: "dotnet", stack: "DOTNETCORE", want: ".NET"},
		{name: "php", stack: "PHP", want: "PHP"},
		{name: "ruby", stack: "RUBY", want: "Ruby"},
		{name: "docker", stack: "DOCKER"},
		{name: "sitecontainers", stack: "SITECONTAINERS"},
		{name: "missing stack"},
		{name: "unknown stack", stack: "unknown"},
		{name: "custom stack is not runtime evidence", stack: "CUSTOM"},
		{name: "worker precedes hosting stack", stack: "SITECONTAINERS", worker: "node"},
		{name: "worker precedes language stack", stack: "DOTNETCORE", worker: "python", want: ".NET"},
		{name: "isolated worker preserved", worker: "dotnet-isolated"},
		{name: "custom worker preserved", worker: " MyWorker "},
		{name: "blank worker", stack: "PYTHON", worker: " \t", want: "Python"},
		{name: "unknown worker", stack: "PYTHON", worker: " UnKnOwN ", want: "Python"},
		{name: "container worker", stack: "DOCKER", worker: " CONTAINER "},
		{name: "null worker", stack: "SITECONTAINERS", worker: " NuLl "},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(WebsiteStack, tt.stack)
			t.Setenv("FUNCTIONS_WORKER_RUNTIME", tt.worker)
			if tt.worker == "" {
				require.NoError(t, os.Unsetenv("FUNCTIONS_WORKER_RUNTIME"))
			}
			// Hosting version metadata must never become an application runtime.
			t.Setenv("DOCKER_SERVER_VERSION", "24.0.0")
			t.Setenv("FUNCTIONS_EXTENSION_VERSION", "~4")
			t.Setenv("WEBSITE_NODE_DEFAULT_VERSION", "")
			t.Setenv("DD_SERVERLESS_INVENTORY_RUNTIME", "")
			service := &AppService{}
			originalTags := service.GetTags()

			expected := []string{tt.worker, tt.want}
			assert.Equal(t, expected, service.GetInventoryData().RuntimeCandidates, "candidates retain raw values and precedence")
			t.Setenv("DD_SERVERLESS_INVENTORY_RUNTIME", "InventoryOnly")
			assert.Equal(t, expected, service.GetInventoryData().RuntimeCandidates, "override belongs to the inventory builder")
			assert.Equal(t, originalTags, service.GetTags(), "inventory derivation and override must not change tracing tags")
			if tt.worker != "" {
				assert.Equal(t, tt.worker, service.GetTags()["aas.environment.runtime"], "tracing retains unmodified worker metadata")
			}
		})
	}
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
	assert.False(t, service.CanCollectInventory())
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
