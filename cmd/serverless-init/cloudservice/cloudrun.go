// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"os"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice/helper"
)

const (
	//nolint:revive // TODO(SERV) Fix revive linter
	revisionNameEnvVar      = "K_REVISION"
	ServiceNameEnvVar       = "K_SERVICE" // ServiceNameEnvVar is also used in the trace package
	configurationNameEnvVar = "K_CONFIGURATION"
	functionTypeEnvVar      = "FUNCTION_SIGNATURE_TYPE"
	functionTargetEnvVar    = "FUNCTION_TARGET" // exists as a cloudrunfunction env var for all runtimes except Go
)

var metadataHelperFunc = helper.GetMetaData
var cloudRunFunctionMode bool

// CloudRun has helper functions for getting Google Cloud Run data
type CloudRun struct{}

// GetTags returns a map of gcp-related tags.
func (c *CloudRun) GetTags() map[string]string {
	tags := metadataHelperFunc(helper.GetDefaultConfig()).TagMap()

	revisionName := os.Getenv(revisionNameEnvVar)
	serviceName := os.Getenv(ServiceNameEnvVar)
	configName := os.Getenv(configurationNameEnvVar)
	functionTarget := os.Getenv(functionTargetEnvVar)

	if revisionName != "" {
		tags["revision_name"] = revisionName
	}

	if serviceName != "" {
		tags["service_name"] = serviceName
	}

	if configName != "" {
		tags["configuration_name"] = configName
	}

	if functionTarget != "" {
		cloudRunFunctionMode = true
		tags["function_target"] = functionTarget
		tags = getFunctionTags(tags)
	}

	tags["origin"] = c.GetOrigin()
	tags["_dd.origin"] = c.GetOrigin()

	return tags
}

func getFunctionTags(tags map[string]string) map[string]string {
	functionSignatureType := os.Getenv(functionTypeEnvVar)
	if functionSignatureType != "" {
		tags["function_signature_type"] = functionSignatureType
	}
	return tags
}

// GetOrigin returns the `origin` attribute type for the given
// cloud service.
func (c *CloudRun) GetOrigin() string {
	if cloudRunFunctionMode {
		return "cloudfunction"
	}
	return "cloudrun"
}

// GetPrefix returns the prefix that we're prefixing all
// metrics with.
func (c *CloudRun) GetPrefix() string {
	if cloudRunFunctionMode {
		return "gcp.cloudfunction"
	}
	return "gcp.run"
}

// Init is empty for CloudRun
func (c *CloudRun) Init() error {
	return nil
}

func isCloudRunService() bool {
	_, exists := os.LookupEnv(ServiceNameEnvVar)
	_, cloudRunFunctionMode = os.LookupEnv(functionTargetEnvVar)
	return exists
}
