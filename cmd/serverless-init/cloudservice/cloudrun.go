// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

// CloudRun has helper functions for getting Google Cloud Run data
type CloudRun struct {
	cloudRunFunctionMode bool
}

// GetTags returns a map of gcp-related tags.
func (c *CloudRun) GetTags() map[string]string {
	tags := metadataHelperFunc(helper.GetDefaultConfig()).TagMap()

	revisionName := os.Getenv(revisionNameEnvVar)
	serviceName := os.Getenv(ServiceNameEnvVar)
	configName := os.Getenv(configurationNameEnvVar)

	if revisionName != "" {
		tags["revision_name"] = revisionName
	}

	if serviceName != "" {
		tags["service_name"] = serviceName
	}

	if configName != "" {
		tags["configuration_name"] = configName
	}

	if c.cloudRunFunctionMode {
		tags = getFunctionTags(tags)
	}
	tags["origin"] = c.GetOrigin()
	tags["_dd.origin"] = c.GetOrigin()

	return tags
}

func getFunctionTags(tags map[string]string) map[string]string {
	functionTarget := os.Getenv(functionTargetEnvVar)
	functionSignatureType := os.Getenv(functionTypeEnvVar)

	if functionTarget != "" {
		tags["function_target"] = functionTarget
	}

	if functionSignatureType != "" {
		tags["function_signature_type"] = functionSignatureType
	}
	return tags
}

// GetOrigin returns the `origin` attribute type for the given
// cloud service.
func (c *CloudRun) GetOrigin() string {
	if c.cloudRunFunctionMode {
		return "cloudfunctions"
	}
	return "cloudrun"
}

// GetPrefix returns the prefix that we're prefixing all
// metrics with.
func (c *CloudRun) GetPrefix() string {
	if c.cloudRunFunctionMode {
		return "gcp.cloudfunctions"
	}
	return "gcp.run"
}

// Init is empty for CloudRun
func (c *CloudRun) Init() error {
	return nil
}

func isCloudRunService() bool {
	_, exists := os.LookupEnv(ServiceNameEnvVar)
	return exists
}

func isCloudRunFunction() bool {
	_, cloudRunFunctionMode := os.LookupEnv(functionTargetEnvVar)
	log.Debug(fmt.Sprintf("cloud function mode SET TO: %t", cloudRunFunctionMode))
	return cloudRunFunctionMode
}
