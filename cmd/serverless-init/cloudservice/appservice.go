// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"os"
)

// AppService has helper functions for getting specific Azure Container App data
type AppService struct{}

const (
	WebsiteName = "WEBSITE_SITE_NAME"
	RegionName  = "REGION_NAME"
	RunZip      = "APPSVC_RUN_ZIP"
)

// GetTags returns a map of Azure-related tags
func (a *AppService) GetTags() map[string]string {
	appName := os.Getenv(WebsiteName)
	region := os.Getenv(RegionName)

	return map[string]string{
		"app_name":   appName,
		"region":     region,
		"origin":     a.GetOrigin(),
		"_dd.origin": a.GetOrigin(),
	}
}

// GetOrigin returns the `origin` attribute type for the given
// cloud service.
func (a *AppService) GetOrigin() string {
	return "appservice"
}

// GetPrefix returns the prefix that we're prefixing all
// metrics with.
func (a *AppService) GetPrefix() string {
	return "azure.appservice"
}

func isAppService() bool {
	_, exists := os.LookupEnv(RunZip)
	return exists
}
