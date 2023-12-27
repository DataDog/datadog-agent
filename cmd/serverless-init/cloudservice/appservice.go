// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package cloudservice

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

// AppService has helper functions for getting specific Azure Container App data
type AppService struct{}

const (
	//nolint:revive // TODO(SERV) Fix revive linter
	WebsiteName = "WEBSITE_SITE_NAME"
	//nolint:revive // TODO(SERV) Fix revive linter
	RegionName = "REGION_NAME"
	//nolint:revive // TODO(SERV) Fix revive linter
	RunZip = "APPSVC_RUN_ZIP"
	//nolint:revive // TODO(SERV) Fix revive linter
	AppLogsTrace = "WEBSITE_APPSERVICEAPPLOGS_TRACE_ENABLED"
)

// GetTags returns a map of Azure-related tags
func (a *AppService) GetTags() map[string]string {
	appName := os.Getenv(WebsiteName)
	region := os.Getenv(RegionName)

	tags := map[string]string{
		"app_name":   appName,
		"region":     region,
		"origin":     a.GetOrigin(),
		"_dd.origin": a.GetOrigin(),
	}

	for key, value := range traceutil.GetAppServicesTags() {
		tags[key] = value
	}

	return tags
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

// Init is empty for AppService
func (a *AppService) Init() error {
	return nil
}

func isAppService() bool {
	_, exists := os.LookupEnv(RunZip)
	return exists
}
