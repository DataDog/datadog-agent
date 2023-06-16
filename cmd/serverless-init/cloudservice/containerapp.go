// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"os"
	"strings"
)

// ContainerApp has helper functions for getting specific Azure Container App data
type ContainerApp struct{}

const (
	ContainerAppNameEnvVar = "CONTAINER_APP_NAME"
	ContainerAppDNSSuffix  = "CONTAINER_APP_ENV_DNS_SUFFIX"
	ContainerAppRevision   = "CONTAINER_APP_REVISION"
)

// GetTags returns a map of Azure-related tags
func (c *ContainerApp) GetTags() map[string]string {
	appName := os.Getenv(ContainerAppNameEnvVar)
	appDNSSuffix := os.Getenv(ContainerAppDNSSuffix)

	appDNSSuffixTokens := strings.Split(appDNSSuffix, ".")
	region := appDNSSuffixTokens[len(appDNSSuffixTokens)-3]

	revision := os.Getenv(ContainerAppRevision)

	return map[string]string{
		"app_name":   appName,
		"region":     region,
		"revision":   revision,
		"origin":     c.GetOrigin(),
		"_dd.origin": c.GetOrigin(),
	}
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

func isContainerAppService() bool {
	_, exists := os.LookupEnv(ContainerAppNameEnvVar)
	return exists
}
