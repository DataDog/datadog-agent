// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package cloudservice

import (
	"maps"
	"os"

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

	appServicePrefix             = "azure.app_services."
	appServiceShutdownMetricName = "azure.app_services.enhanced.shutdown"
	appServiceStartMetricName    = "azure.app_services.enhanced.cold_start"

	appServiceLegacyShutdownMetricName = "azure.appservice.enhanced.shutdown"
	appServiceLegacyStartMetricName    = "azure.appservice.enhanced.cold_start"

	appServiceUsageMetricSuffix = "instance"
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

func (a *AppService) GetEnhancedMetricTags(tags map[string]string) EnhancedMetricTags {
	baseTags := map[string]string{
		"name":            tagValueOrUnknown(tags["app_name"]),
		"origin":          tagValueOrUnknown(tags["origin"]),
		"region":          tagValueOrUnknown(tags["region"]),
		"resource_group":  tagValueOrUnknown(tags["aas.resource.group"]),
		"subscription_id": tagValueOrUnknown(tags["aas.subscription.id"]),
	}

	usageTags := maps.Clone(baseTags)
	usageTags["instance"] = tagValueOrUnknown(tags["aas.environment.instance_name"])

	return EnhancedMetricTags{Base: baseTags, Usage: usageTags}
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

func (a *AppService) GetMetricPrefix() string {
	return appServicePrefix
}

func (a *AppService) GetUsageMetricSuffix() string {
	return appServiceUsageMetricSuffix
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
func (a *AppService) Shutdown(metricAgent serverlessMetrics.ServerlessMetricAgent, enhancedMetricsEnabled bool, _ error) {
	if enhancedMetricsEnabled {
		metricAgent.AddEnhancedMetric(appServiceShutdownMetricName, 1.0, a.GetSource(), 0)
		metricAgent.AddLegacyEnhancedMetric(appServiceLegacyShutdownMetricName, 1.0, a.GetSource())
	}
}

func (a *AppService) AddStartMetric(metricAgent *serverlessMetrics.ServerlessMetricAgent) {
	metricAgent.AddEnhancedMetric(appServiceStartMetricName, 1.0, a.GetSource(), 0)
	metricAgent.AddLegacyEnhancedMetric(appServiceLegacyStartMetricName, 1.0, a.GetSource())
}

// ShouldForceFlushAllOnForceFlushToSerializer is false usually.
func (a *AppService) ShouldForceFlushAllOnForceFlushToSerializer() bool {
	return false
}

func isAppService() bool {
	_, exists := os.LookupEnv(WebsiteStack)
	return exists
}
