// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"fmt"
	"os"
	"strings"
)

// ContainerApp has helper functions for getting specific Azure Container App data
type ContainerApp struct {
	SubscriptionId string
	ResourceGroup  string
}

const (
	ContainerAppNameEnvVar = "CONTAINER_APP_NAME"
	ContainerAppDNSSuffix  = "CONTAINER_APP_ENV_DNS_SUFFIX"
	ContainerAppRevision   = "CONTAINER_APP_REVISION"

	AzureSubscriptionIdEnvVar = "DD_AZURE_SUBSCRIPTION_ID"
	AzureResourceGroupEnvVar  = "DD_AZURE_RESOURCE_GROUP"
)

// GetTags returns a map of Azure-related tags
func (c *ContainerApp) GetTags() map[string]string {
	appName := os.Getenv(ContainerAppNameEnvVar)
	appDNSSuffix := os.Getenv(ContainerAppDNSSuffix)

	appDNSSuffixTokens := strings.Split(appDNSSuffix, ".")
	region := appDNSSuffixTokens[len(appDNSSuffixTokens)-3]

	revision := os.Getenv(ContainerAppRevision)

	tags := map[string]string{
		"app_name":   appName,
		"region":     region,
		"revision":   revision,
		"origin":     c.GetOrigin(),
		"_dd.origin": c.GetOrigin(),
	}

	if c.SubscriptionId != "" {
		tags["subscription_id"] = c.SubscriptionId
	}

	if c.ResourceGroup != "" {
		tags["resource_group"] = c.ResourceGroup
	}

	return tags
}

// GetOrigin returns the `origin` attribute type for the given
// cloud service.
func (c *ContainerApp) GetOrigin() string {
	return "containerapp"
}

// GetPrefix returns the prefix that we're prefixing all
// metrics with.
func (c *ContainerApp) GetPrefix() string {
	return "azure.containerapp"
}

// NewContainerApp returns a new ContainerApp instance
func NewContainerApp() *ContainerApp {
	return &ContainerApp{
		SubscriptionId: "",
		ResourceGroup:  "",
	}
}

// Init initializes ContainerApp specific code
func (c *ContainerApp) Init() error {
	// For ContainerApp, the customers must set DD_AZURE_SUBSCRIPTION_ID
	// and DD_AZURE_RESOURCE_GROUP.
	// These environment variables are optional for now. Once we go GA,
	// return `false` if these are not set.
	if subscriptionId, exists := os.LookupEnv(AzureSubscriptionIdEnvVar); exists {
		c.SubscriptionId = subscriptionId
	} else {
		return fmt.Errorf("must set %v", AzureSubscriptionIdEnvVar)
	}

	if resourceGroup, exists := os.LookupEnv(AzureResourceGroupEnvVar); exists {
		c.ResourceGroup = resourceGroup
	} else {
		return fmt.Errorf("must set %v", AzureResourceGroupEnvVar)
	}

	return nil
}

func isContainerAppService() bool {
	_, exists := os.LookupEnv(ContainerAppNameEnvVar)
	return exists
}
