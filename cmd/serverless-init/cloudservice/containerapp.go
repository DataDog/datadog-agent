// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/metric"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
)

// ContainerApp has helper functions for getting specific Azure Container App data
type ContainerApp struct {
	//nolint:revive // TODO(SERV) Fix revive linter
	SubscriptionId string
	//nolint:revive // TODO(SERV) Fix revive linter
	ResourceGroup string
}

const (
	//nolint:revive // TODO(SERV) Fix revive linter
	ContainerAppNameEnvVar = "CONTAINER_APP_NAME"
	//nolint:revive // TODO(SERV) Fix revive linter
	ContainerAppDNSSuffix = "CONTAINER_APP_ENV_DNS_SUFFIX"
	//nolint:revive // TODO(SERV) Fix revive linter
	ContainerAppRevision = "CONTAINER_APP_REVISION"
	//nolint:revive // TODO(SERV) Fix revive linter
	ContainerAppReplicaName = "CONTAINER_APP_REPLICA_NAME"

	//nolint:revive // TODO(SERV) Fix revive linter
	AzureSubscriptionIdEnvVar = "DD_AZURE_SUBSCRIPTION_ID"
	//nolint:revive // TODO(SERV) Fix revive linter
	AzureResourceGroupEnvVar = "DD_AZURE_RESOURCE_GROUP"

	// updated tag namespace for new billing
	acaReplicaName    = "aca.replica.name"
	acaResourceID     = "aca.resource.id"
	acaResourceGroup  = "aca.resource.group"
	acaAppName        = "aca.app.name"
	acaRegion         = "aca.app.region"
	acaRevision       = "aca.app.revision"
	acaSubscriptionID = "aca.subscription.id"

	// ContainerAppOrigin origin tag value
	ContainerAppOrigin = "containerapp"

	containerAppPrefix = "azure.containerapp"
)

// GetTags returns a map of Azure-related tags
func (c *ContainerApp) GetTags() map[string]string {
	appName := os.Getenv(ContainerAppNameEnvVar)
	appDNSSuffix := os.Getenv(ContainerAppDNSSuffix)

	appDNSSuffixTokens := strings.Split(appDNSSuffix, ".")
	region := appDNSSuffixTokens[len(appDNSSuffixTokens)-3]

	revision := os.Getenv(ContainerAppRevision)
	replica := os.Getenv(ContainerAppReplicaName)

	// Check ContainerApp struct first, then fall back to environment variables
	subscriptionID := c.SubscriptionId
	if subscriptionID == "" {
		subscriptionID = os.Getenv(AzureSubscriptionIdEnvVar)
	}

	resourceGroup := c.ResourceGroup
	if resourceGroup == "" {
		resourceGroup = os.Getenv(AzureResourceGroupEnvVar)
	}

	// There are some duplicate tags here because we are updating billing and adding
	// an abbreviated namespace per Azure environment. We must maintain backwards
	// compatibility. An example is "replica_name" and "aca.replica.name"
	tags := map[string]string{
		"app_name":     appName,
		"region":       region,
		"revision":     revision,
		"replica_name": replica,
		// new tags
		acaAppName:     appName,
		acaRegion:      region,
		acaRevision:    revision,
		acaReplicaName: replica,

		"origin":     ContainerAppOrigin,
		"_dd.origin": ContainerAppOrigin,
	}

	if subscriptionID != "" {
		tags["subscription_id"] = subscriptionID
		tags[acaSubscriptionID] = subscriptionID
	}

	if resourceGroup != "" {
		tags["resource_group"] = resourceGroup
		tags[acaResourceGroup] = resourceGroup
	}

	if subscriptionID != "" && resourceGroup != "" {
		resourceID := fmt.Sprintf("/subscriptions/%v/resourcegroups/%v/providers/microsoft.app/containerapps/%v", subscriptionID, resourceGroup, strings.ToLower(appName))
		tags["resource_id"] = resourceID
		tags[acaResourceID] = resourceID

	}

	return tags
}

// GetDefaultLogsSource returns the default logs source if `DD_SOURCE` is not set
func (c *ContainerApp) GetDefaultLogsSource() string {
	return ContainerAppOrigin
}

// GetOrigin returns the `origin` attribute type for the given
// cloud service.
func (c *ContainerApp) GetOrigin() string {
	return ContainerAppOrigin
}

// GetSource returns the metrics source
func (c *ContainerApp) GetSource() metrics.MetricSource {
	return metrics.MetricSourceAzureContainerAppEnhanced
}

// NewContainerApp returns a new ContainerApp instance
func NewContainerApp() *ContainerApp {
	return &ContainerApp{
		SubscriptionId: "",
		ResourceGroup:  "",
	}
}

// Init initializes ContainerApp specific code
func (c *ContainerApp) Init(_ *TracingContext) error {
	// For ContainerApp, the customers must set DD_AZURE_SUBSCRIPTION_ID
	// and DD_AZURE_RESOURCE_GROUP.
	// These environment variables are optional for now. Once we go GA,
	// return an error if these are not set.
	//nolint:revive // TODO(SERV) Fix revive linter
	if subscriptionId, exists := os.LookupEnv(AzureSubscriptionIdEnvVar); exists {
		c.SubscriptionId = subscriptionId
	} else {
		log.Fatalf("Must set Subscription ID as an environment variable. Please set the %v value to your Subscription ID your App Container is in.", AzureSubscriptionIdEnvVar)
	}

	if resourceGroup, exists := os.LookupEnv(AzureResourceGroupEnvVar); exists {
		c.ResourceGroup = resourceGroup
	} else {
		log.Fatalf("Must set Resource Group as an environment variable. Please set the %v value to your Resource Group your App Container is in.", AzureResourceGroupEnvVar)
	}

	return nil
}

// Shutdown emits the shutdown metric for ContainerApp
func (c *ContainerApp) Shutdown(metricAgent serverlessMetrics.ServerlessMetricAgent, _ error) {
	metric.Add(containerAppPrefix+".enhanced.shutdown", 1.0, c.GetSource(), metricAgent)
}

// GetStartMetricName returns the metric name for container start (coldstart) events
func (c *ContainerApp) GetStartMetricName() string {
	return containerAppPrefix + ".enhanced.cold_start"
}

// ShouldForceFlushAllOnForceFlushToSerializer is false usually.
func (c *ContainerApp) ShouldForceFlushAllOnForceFlushToSerializer() bool {
	return false
}

func isContainerAppService() bool {
	_, exists := os.LookupEnv(ContainerAppNameEnvVar)
	return exists
}
