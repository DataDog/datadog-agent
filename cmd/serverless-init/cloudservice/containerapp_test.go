// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	serverlessdef "github.com/DataDog/datadog-agent/comp/metadata/serverlessinit/def"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
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

func TestContainerAppGetMetadata(t *testing.T) {
	service := NewContainerApp()

	t.Setenv("CONTAINER_APP_NAME", "Test_App_Name")
	t.Setenv("CONTAINER_APP_ENV_DNS_SUFFIX", "test.bluebeach.eastus.azurecontainerapps.io")
	t.Setenv("CONTAINER_APP_REVISION", "Test_Revision")
	t.Setenv("CONTAINER_APP_REPLICA_NAME", "Test_Replica")
	t.Setenv("DD_AZURE_SUBSCRIPTION_ID", "Test_Subscription_ID")
	t.Setenv("DD_AZURE_RESOURCE_GROUP", "Test_Resource_Group")

	metadata := service.GetMetadata()

	appCCRID := "/subscriptions/test_subscription_id/resourcegroups/test_resource_group/providers/microsoft.app/containerapps/test_app_name"
	assert.Equal(t, serverlessdef.CloudServiceMetadata{
		WorkloadType:        workloadTypeAzureContainerApp,
		ResourceID:          appCCRID + "/revisions/Test_Revision",
		ParentResourceID:    appCCRID,
		ResourceName:        "Test_App_Name",
		Region:              "eastus",
		AzureSubscriptionID: "Test_Subscription_ID",
		AzureResourceGroup:  "Test_Resource_Group",
	}, metadata)
	assert.True(t, service.CanCollectMetadata())

	tags := service.GetTags()
	assert.Equal(t, map[string]string{
		"app_name":            "Test_App_Name",
		"origin":              "containerapp",
		"region":              "eastus",
		"revision":            "Test_Revision",
		"replica_name":        "Test_Replica",
		"_dd.origin":          "containerapp",
		"subscription_id":     "Test_Subscription_ID",
		"resource_id":         "/subscriptions/Test_Subscription_ID/resourcegroups/Test_Resource_Group/providers/microsoft.app/containerapps/test_app_name",
		"resource_group":      "Test_Resource_Group",
		"aca.app.name":        "Test_App_Name",
		"aca.app.region":      "eastus",
		"aca.app.revision":    "Test_Revision",
		"aca.replica.name":    "Test_Replica",
		"aca.resource.id":     "/subscriptions/Test_Subscription_ID/resourcegroups/Test_Resource_Group/providers/microsoft.app/containerapps/test_app_name",
		"aca.resource.group":  "Test_Resource_Group",
		"aca.subscription.id": "Test_Subscription_ID",
	}, tags)
	assert.Equal(t, map[string]string{
		"name":            "Test_App_Name",
		"origin":          "containerapp",
		"region":          "eastus",
		"resource_group":  "Test_Resource_Group",
		"revisionname":    "Test_Revision",
		"subscription_id": "Test_Subscription_ID",
	}, service.GetEnhancedMetricTags(tags).Base)
}

func TestContainerAppGetMetadataWithoutAzureIDs(t *testing.T) {
	service := NewContainerApp()

	t.Setenv("CONTAINER_APP_NAME", "test_app_name")
	t.Setenv("CONTAINER_APP_ENV_DNS_SUFFIX", "test.bluebeach.eastus.azurecontainerapps.io")
	t.Setenv("CONTAINER_APP_REVISION", "test_revision")
	os.Unsetenv("DD_AZURE_SUBSCRIPTION_ID")
	os.Unsetenv("DD_AZURE_RESOURCE_GROUP")

	assert.Equal(t, serverlessdef.CloudServiceMetadata{
		WorkloadType: workloadTypeAzureContainerApp,
		ResourceName: "test_app_name",
		Region:       "eastus",
	}, service.GetMetadata())
}

func TestContainerAppGetMetadataMalformedDNSSuffix(t *testing.T) {
	service := NewContainerApp()
	t.Setenv("CONTAINER_APP_NAME", "test_app")
	t.Setenv("CONTAINER_APP_ENV_DNS_SUFFIX", "foo.bar")
	t.Setenv("CONTAINER_APP_REVISION", "test_revision")
	t.Setenv("DD_AZURE_SUBSCRIPTION_ID", "test_subscription_id")
	t.Setenv("DD_AZURE_RESOURCE_GROUP", "test_resource_group")

	metadata := service.GetMetadata()

	assert.Empty(t, metadata.Region, "metadata leaves an unresolved region empty")
	assert.NotEmpty(t, metadata.ResourceID)
	assert.Equal(t, "unknown", service.GetTags()["region"])
}

func TestContainerAppReadsCurrentEnvironment(t *testing.T) {
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
	assert.Equal(t, "/subscriptions/next-subscription/resourcegroups/next-group/providers/microsoft.app/containerapps/next-app/revisions/Next-Revision", service.GetMetadata().ResourceID)
	assert.Equal(t, tags, service.GetTags(), "metadata derivation must not alter telemetry tags")

	t.Setenv(AzureSubscriptionIdEnvVar, "")
	tags = service.GetTags()
	assert.NotContains(t, tags, "subscription_id")
	assert.NotContains(t, tags, acaSubscriptionID)
	assert.NotContains(t, tags, "resource_id")
	assert.NotContains(t, tags, acaResourceID)
	assert.Equal(t, "unknown", service.GetEnhancedMetricTags(tags).Base["subscription_id"])
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

func TestGetContainerAppTagsEmptyDNSSuffix(t *testing.T) {
	service := NewContainerApp()
	t.Setenv("CONTAINER_APP_NAME", "test_app")
	t.Setenv("CONTAINER_APP_ENV_DNS_SUFFIX", "")
	t.Setenv("CONTAINER_APP_REVISION", "test_revision")
	t.Setenv("CONTAINER_APP_REPLICA_NAME", "test--replica")

	tags := service.GetTags()

	assert.Equal(t, "unknown", tags["region"])
	assert.Equal(t, "unknown", tags[acaRegion])
}

func TestGetContainerAppTagsShortDNSSuffix(t *testing.T) {
	service := NewContainerApp()
	t.Setenv("CONTAINER_APP_NAME", "test_app")
	t.Setenv("CONTAINER_APP_ENV_DNS_SUFFIX", "foo.bar")
	t.Setenv("CONTAINER_APP_REVISION", "test_revision")
	t.Setenv("CONTAINER_APP_REPLICA_NAME", "test--replica")

	tags := service.GetTags()

	assert.Equal(t, "unknown", tags["region"])
	assert.Equal(t, "unknown", tags[acaRegion])
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

func TestContainerAppMissingIdentity(t *testing.T) {
	for _, missing := range []string{"none", AzureSubscriptionIdEnvVar, AzureResourceGroupEnvVar, ContainerAppNameEnvVar, ContainerAppRevision} {
		t.Run(missing, func(t *testing.T) {
			t.Setenv(AzureSubscriptionIdEnvVar, "subscription")
			t.Setenv(AzureResourceGroupEnvVar, "resource group")
			t.Setenv(ContainerAppNameEnvVar, "unknown")
			t.Setenv(ContainerAppRevision, "revision")
			t.Setenv(ContainerAppReplicaName, "")
			t.Setenv(ContainerAppDNSSuffix, "")
			if missing != "none" {
				t.Setenv(missing, "")
			}
			service := NewContainerApp()
			if missing == "none" {
				assert.NotEmpty(t, service.GetMetadata().ResourceID)
			} else {
				assert.Empty(t, service.GetMetadata().ResourceID)
			}
			assert.True(t, service.CanCollectMetadata(), "identity readiness does not change platform support")
			if missing != "none" && missing != ContainerAppRevision {
				assert.NotContains(t, service.GetTags(), "resource_id")
			}
		})
	}
}

func TestContainerAppShutdownEmitsMetrics(t *testing.T) {
	skipOnWindows(t)
	demux := createDemultiplexer(t)
	agent := &serverlessMetrics.ServerlessMetricAgent{Demux: demux}

	service := NewContainerApp()
	service.Shutdown(agent, true, nil)

	generatedMetrics, timedMetrics := demux.WaitForSamples(100 * time.Millisecond)
	assert.Empty(t, timedMetrics)
	assert.Len(t, generatedMetrics, 2)

	foundShutdown := false
	for _, sample := range generatedMetrics {
		if sample.Name == containerAppShutdownMetricName {
			foundShutdown = true
		}
	}
	assert.True(t, foundShutdown, "shutdown metric not emitted")
}

func TestContainerAppShutdownNilMetricAgent(t *testing.T) {
	service := NewContainerApp()
	require.NotPanics(t, func() {
		service.Shutdown(nil, true, nil)
	})
}
