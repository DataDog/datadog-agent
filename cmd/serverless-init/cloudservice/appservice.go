// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package cloudservice

import (
	"maps"
	"os"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/metric"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
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
	WebsiteStack = "WEBSITE_STACK"
	//nolint:revive // TODO(SERV) Fix revive linter
	AppLogsTrace = "WEBSITE_APPSERVICEAPPLOGS_TRACE_ENABLED"

	// AppServiceOrigin origin tag value
	AppServiceOrigin = "appservice"

	appServicePrefix = "azure.appservice"
)

// GetTags returns a map of Azure-related tags
func (a *AppService) GetTags() map[string]string {
	appName := os.Getenv(WebsiteName)
	region := os.Getenv(RegionName)

	tags := map[string]string{
		"app_name":   appName,
		"region":     region,
		"origin":     AppServiceOrigin,
		"_dd.origin": AppServiceOrigin,
	}

	maps.Copy(tags, traceutil.GetAppServicesTags())

	return tags
}

// GetDefaultLogsSource returns the default logs source if `DD_SOURCE` is not set
func (a *AppService) GetDefaultLogsSource() string {
	return AppServiceOrigin
}

// GetOrigin returns the `origin` attribute type for the given
// cloud service.
func (a *AppService) GetOrigin() string {
	return AppServiceOrigin
}

// GetSource returns the metrics source
func (a *AppService) GetSource() metrics.MetricSource {
	return metrics.MetricSourceAzureAppServiceEnhanced
}

// Init is empty for AppService
func (a *AppService) Init(_ *TracingContext) error {
	return nil
}

// Shutdown emits the shutdown metric for AppService
func (a *AppService) Shutdown(metricAgent serverlessMetrics.ServerlessMetricAgent, _ error) {
	metric.Add(appServicePrefix+".enhanced.shutdown", 1.0, a.GetSource(), metricAgent)
}

// GetStartMetricName returns the metric name for container start (coldstart) events
func (a *AppService) GetStartMetricName() string {
	return appServicePrefix + ".enhanced.cold_start"
}

// ShouldForceFlushAllOnForceFlushToSerializer is false usually.
func (a *AppService) ShouldForceFlushAllOnForceFlushToSerializer() bool {
	return false
}

func isAppService() bool {
	_, exists := os.LookupEnv(WebsiteStack)
	return exists
}
