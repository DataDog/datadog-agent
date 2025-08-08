// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package env provides serverless environment detection utilities
package env

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// AzureAppServicesEnvVar is set to "1" when running in Datadog Azure App
	// Services extension
	AzureAppServicesEnvVar = "DD_AZURE_APP_SERVICES"
)

// IsAzureAppServicesExtension returns true if running in Datadog Azure App
// Services extension context
func IsAzureAppServicesExtension() bool {
	value, exists := os.LookupEnv(AzureAppServicesEnvVar)
	result := exists && value == "1"

	if result {
		log.Debug("Running in an Azure App Services Extension")
	}

	return result
}
