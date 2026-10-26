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

	serverlessdef "github.com/DataDog/datadog-agent/comp/metadata/serverlessinit/def"
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

func TestAppServiceGetMetadata(t *testing.T) {
	service := &AppService{}

	t.Setenv("WEBSITE_SITE_NAME", "Test_Site_Name")
	t.Setenv("REGION_NAME", "eastus")
	t.Setenv("WEBSITE_OWNER_NAME", "Test_Subscription_ID+resourcegroup-EastUSwebspace")
	t.Setenv("WEBSITE_RESOURCE_GROUP", "Test_Resource_Group")
	t.Setenv("WEBSITE_STACK", "NODE")
	t.Setenv("WEBSITE_NODE_DEFAULT_VERSION", "~18")
	t.Setenv("FUNCTIONS_WORKER_RUNTIME", "")
	require.NoError(t, os.Unsetenv("FUNCTIONS_WORKER_RUNTIME"))

	inv := service.GetMetadata()

	assert.Equal(t, serverlessdef.CloudServiceMetadata{
		WorkloadType:        workloadTypeAzureAppService,
		ResourceID:          "/subscriptions/test_subscription_id/resourcegroups/test_resource_group/providers/microsoft.web/sites/test_site_name",
		ResourceName:        "Test_Site_Name",
		Region:              "eastus",
		AzureSubscriptionID: "Test_Subscription_ID",
		AzureResourceGroup:  "Test_Resource_Group",
		RuntimeCandidates:   []string{"", "Node.js"},
	}, inv)
	assert.True(t, service.CanCollectMetadata())
	assert.Empty(t, inv.ParentResourceID)

	tags := service.GetTags()
	assert.Equal(t, "/subscriptions/Test_Subscription_ID/resourcegroups/Test_Resource_Group/providers/microsoft.web/sites/Test_Site_Name", tags["aas.resource.id"])
	assert.Equal(t, "Test_Site_Name", tags["app_name"])
	assert.Equal(t, "Test_Site_Name", tags["aas.site.name"])
	assert.Equal(t, "Test_Subscription_ID", tags["aas.subscription.id"])
	assert.Equal(t, "Test_Resource_Group", tags["aas.resource.group"])
	assert.Equal(t, map[string]string{
		"name":            "Test_Site_Name",
		"origin":          "appservice",
		"region":          "eastus",
		"resource_group":  "Test_Resource_Group",
		"subscription_id": "Test_Subscription_ID",
	}, service.GetEnhancedMetricTags(tags).Base)
}

func TestAppServiceGetMetadataFunctionApp(t *testing.T) {
	for _, worker := range []string{"node", ""} {
		t.Run("worker="+worker, func(t *testing.T) {
			service := &AppService{}

			t.Setenv("WEBSITE_SITE_NAME", "Test_Site_Name")
			t.Setenv("REGION_NAME", "eastus")
			t.Setenv("WEBSITE_OWNER_NAME", "Test_Subscription_ID+resourcegroup-EastUSwebspace")
			t.Setenv("WEBSITE_RESOURCE_GROUP", "Test_Resource_Group")
			t.Setenv("WEBSITE_STACK", "NODE")
			t.Setenv("FUNCTIONS_WORKER_RUNTIME", worker)

			inv := service.GetMetadata()

			assert.True(t, service.CanCollectMetadata())
			assert.Equal(t, serverlessdef.CloudServiceMetadata{
				WorkloadType:        workloadTypeAzureFunction,
				ResourceID:          "/subscriptions/Test_Subscription_ID/resourcegroups/Test_Resource_Group/providers/microsoft.web/sites/Test_Site_Name",
				ResourceName:        "Test_Site_Name",
				Region:              "eastus",
				AzureSubscriptionID: "Test_Subscription_ID",
				AzureResourceGroup:  "Test_Resource_Group",
				RuntimeCandidates:   []string{worker, "Node.js"},
			}, inv)
			tags := service.GetTags()
			assert.Equal(t, inv.ResourceID, tags["aas.resource.id"])
			assert.Equal(t, "functionapp", tags["aas.site.kind"])
			assert.Equal(t, worker, tags["aas.environment.runtime"])
		})
	}
}

func TestAppServiceMissingIdentity(t *testing.T) {
	for _, missing := range []string{"none", "WEBSITE_OWNER_NAME", "WEBSITE_RESOURCE_GROUP", WebsiteName} {
		t.Run(missing, func(t *testing.T) {
			t.Setenv("WEBSITE_OWNER_NAME", "Test_Subscription_ID+resourcegroup-EastUSwebspace")
			t.Setenv("WEBSITE_RESOURCE_GROUP", "Test_Resource_Group")
			t.Setenv(WebsiteName, "Test_Site_Name")
			t.Setenv(RegionName, "")
			t.Setenv("FUNCTIONS_WORKER_RUNTIME", "")
			require.NoError(t, os.Unsetenv("FUNCTIONS_WORKER_RUNTIME"))
			if missing != "none" {
				t.Setenv(missing, "")
			}

			service := &AppService{}
			metadata := service.GetMetadata()
			assert.Empty(t, metadata.ParentResourceID)
			if missing == "none" {
				assert.NotEmpty(t, metadata.ResourceID)
			} else {
				assert.Empty(t, metadata.ResourceID)
				assert.Empty(t, service.GetTags()["aas.resource.id"])
			}
			assert.True(t, service.CanCollectMetadata(), "identity readiness does not change platform support")
		})
	}
}

func TestAppServiceMetadataPreservesTelemetry(t *testing.T) {
	for _, worker := range []string{"", "python"} {
		t.Run("worker="+worker, func(t *testing.T) {
			t.Setenv("WEBSITE_SITE_NAME", "Test-Site")
			t.Setenv("WEBSITE_OWNER_NAME", "Test-Subscription+webspace")
			t.Setenv("WEBSITE_RESOURCE_GROUP", "Test-Group")
			t.Setenv("REGION_NAME", "eastus")
			t.Setenv("WEBSITE_STACK", "NODE")
			t.Setenv("WEBSITE_NODE_DEFAULT_VERSION", "~18")
			t.Setenv("COMPUTERNAME", "Test-Instance")
			t.Setenv("FUNCTIONS_WORKER_RUNTIME", worker)
			if worker == "" {
				require.NoError(t, os.Unsetenv("FUNCTIONS_WORKER_RUNTIME"))
			}
			service := &AppService{}
			tags := service.GetTags()
			assert.Equal(t, "/subscriptions/Test-Subscription/resourcegroups/Test-Group/providers/microsoft.web/sites/Test-Site", tags["aas.resource.id"])
			if worker == "" {
				assert.Equal(t, "Node.js", tags["aas.environment.runtime"])
			} else {
				assert.Equal(t, worker, tags["aas.environment.runtime"])
			}
			enhanced := service.GetEnhancedMetricTags(tags)
			assert.Equal(t, map[string]string{
				"name": "Test-Site", "origin": "appservice", "region": "eastus",
				"resource_group": "Test-Group", "subscription_id": "Test-Subscription",
			}, enhanced.Base)
			assert.Equal(t, "Test-Instance", enhanced.Usage["instance"])

			service.GetMetadata()
			assert.Equal(t, tags, service.GetTags())
			assert.Equal(t, enhanced, service.GetEnhancedMetricTags(service.GetTags()))
		})
	}
}

func TestAppServiceMetadataRuntimeCandidates(t *testing.T) {
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
			service := &AppService{}
			originalTags := service.GetTags()

			expected := []string{tt.worker, tt.want}
			assert.Equal(t, expected, service.GetMetadata().RuntimeCandidates, "candidates retain raw values and precedence")
			assert.Equal(t, originalTags, service.GetTags(), "metadata derivation must not change tracing tags")
			if tt.worker != "" {
				assert.Equal(t, tt.worker, service.GetTags()["aas.environment.runtime"], "tracing retains unmodified worker metadata")
			}
		})
	}
}

func TestAppServiceGetMetadataWithoutAzureIDs(t *testing.T) {
	service := &AppService{}

	t.Setenv("WEBSITE_SITE_NAME", "test_site_name")
	t.Setenv("REGION_NAME", "eastus")
	os.Unsetenv("WEBSITE_OWNER_NAME")
	os.Unsetenv("WEBSITE_RESOURCE_GROUP")
	os.Unsetenv("FUNCTIONS_WORKER_RUNTIME")

	inv := service.GetMetadata()

	assert.Equal(t, workloadTypeAzureAppService, inv.WorkloadType)
	assert.Empty(t, inv.ResourceID)
	assert.True(t, service.CanCollectMetadata(), "identity readiness does not change platform eligibility")
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
