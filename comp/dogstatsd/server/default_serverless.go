// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless

package server

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const (
	googleCloudRunServiceNameEnvVar = "K_SERVICE"
	azureContainerAppNameEnvVar     = "CONTAINER_APP_NAME"
	azureAppServiceNameEnvVar       = "WEBSITE_STACK"
)

// GetDefaultMetricSource returns the default metric source based on build tags
func GetDefaultMetricSource() metrics.MetricSource {
	if _, ok := os.LookupEnv(googleCloudRunServiceNameEnvVar); ok {
		return metrics.MetricSourceGoogleCloudRunCustom
	}
	if _, ok := os.LookupEnv(azureContainerAppNameEnvVar); ok {
		return metrics.MetricSourceAzureContainerAppCustom
	}
	if _, ok := os.LookupEnv(azureAppServiceNameEnvVar); ok {
		return metrics.MetricSourceAzureAppServiceCustom
	}

	return metrics.MetricSourceServerless
}
