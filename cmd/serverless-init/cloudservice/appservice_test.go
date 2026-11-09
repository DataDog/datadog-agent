// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"os"
	"strings"
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
	const armID = "/subscriptions/Test_Subscription_ID/resourcegroups/Test_Resource_Group/providers/microsoft.web/sites/Test_Site_Name"
	for _, tt := range []struct {
		name         string
		functionApp  bool
		worker       string
		workloadType string
		resourceID   string
	}{
		{name: "web app", workloadType: workloadTypeAzureAppService, resourceID: strings.ToLower(armID)},
		{name: "function app", functionApp: true, worker: "node", workloadType: workloadTypeAzureFunction, resourceID: armID},
		{name: "function app with empty worker", functionApp: true, workloadType: workloadTypeAzureFunction, resourceID: armID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("WEBSITE_SITE_NAME", "Test_Site_Name")
			t.Setenv("REGION_NAME", "eastus")
			t.Setenv("WEBSITE_OWNER_NAME", "Test_Subscription_ID+resourcegroup-EastUSwebspace")
			t.Setenv("WEBSITE_RESOURCE_GROUP", "Test_Resource_Group")
			t.Setenv("WEBSITE_STACK", "NODE")
			t.Setenv("FUNCTIONS_WORKER_RUNTIME", tt.worker)
			if !tt.functionApp {
				require.NoError(t, os.Unsetenv("FUNCTIONS_WORKER_RUNTIME"))
			}
			service := &AppService{}

			assert.Equal(t, serverlessdef.CloudServiceMetadata{
				WorkloadType:        tt.workloadType,
				ResourceID:          tt.resourceID,
				ResourceName:        "Test_Site_Name",
				Region:              "eastus",
				AzureSubscriptionID: "Test_Subscription_ID",
				AzureResourceGroup:  "Test_Resource_Group",
				RuntimeCandidates:   []string{tt.worker, "Node.js"},
			}, service.GetMetadata())
			assert.Equal(t, armID, service.GetTags()["aas.resource.id"], "tags keep the ARM ID casing")
			assert.True(t, service.CanCollectMetadata())
		})
	}
}

func TestAppServiceMissingIdentity(t *testing.T) {
	for _, missing := range []string{"WEBSITE_OWNER_NAME", "WEBSITE_RESOURCE_GROUP", WebsiteName} {
		t.Run(missing, func(t *testing.T) {
			t.Setenv("WEBSITE_OWNER_NAME", "Test_Subscription_ID+resourcegroup-EastUSwebspace")
			t.Setenv("WEBSITE_RESOURCE_GROUP", "Test_Resource_Group")
			t.Setenv(WebsiteName, "Test_Site_Name")
			t.Setenv(missing, "")

			assert.Empty(t, (&AppService{}).GetMetadata().ResourceID)
		})
	}
}

func TestAppServiceMetadataRuntimeCandidates(t *testing.T) {
	for _, tt := range []struct {
		stack string
		want  string
	}{
		{stack: "NODE", want: "Node.js"},
		{stack: "PYTHON", want: "Python"},
		{stack: "JAVA", want: "Java"},
		{stack: "TOMCAT", want: "Java"},
		{stack: "DOTNETCORE", want: ".NET"},
		{stack: "PHP", want: "PHP"},
		{stack: "RUBY", want: "Ruby"},
		{stack: "DOCKER"},
		{stack: "SITECONTAINERS"},
		{stack: ""},
	} {
		t.Run("stack="+tt.stack, func(t *testing.T) {
			t.Setenv(WebsiteStack, tt.stack)
			t.Setenv("FUNCTIONS_WORKER_RUNTIME", " MyWorker ")

			assert.Equal(t, []string{" MyWorker ", tt.want}, (&AppService{}).GetMetadata().RuntimeCandidates,
				"the worker comes first and keeps its raw value")
		})
	}
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
