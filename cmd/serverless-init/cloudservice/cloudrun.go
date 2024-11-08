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
	revisionNameEnvVar      = "K_REVISION"
	ServiceNameEnvVar       = "K_SERVICE" // ServiceNameEnvVar is also used in the trace package
	configurationNameEnvVar = "K_CONFIGURATION"
	functionTypeEnvVar      = "FUNCTION_SIGNATURE_TYPE"
	functionTargetEnvVar    = "FUNCTION_TARGET" // exists as a cloudrunfunction env var for all runtimes except Go
)

const (
	// SpanNamespace is the namespace for the span
	cloudRunService  = "gcr."
	cloudRunFunction = "gcrfx."
)

var metadataHelperFunc = helper.GetMetaData

// CloudRun has helper functions for getting Google Cloud Run data
type CloudRun struct {
	spanNamespace        string
	cloudRunFunctionMode bool
}

// GetTags returns a map of gcp-related tags.
func (c *CloudRun) GetTags() map[string]string {
	tags := metadataHelperFunc(helper.GetDefaultConfig()).TagMap(c.spanNamespace)
	revisionName := os.Getenv(revisionNameEnvVar)
	serviceName := os.Getenv(ServiceNameEnvVar)
	configName := os.Getenv(configurationNameEnvVar)

	if revisionName != "" {
		tags[c.spanNamespace+"revision_name"] = revisionName
		tags["revision_name"] = revisionName
	}

	if serviceName != "" {
		tags[c.spanNamespace+"service_name"] = serviceName
		tags["service_name"] = serviceName
	}

	if configName != "" {
		tags[c.spanNamespace+"configuration_name"] = configName
		tags["configuration_name"] = configName
	}

	if c.cloudRunFunctionMode {
		tags = c.getFunctionTags(tags)
	}

	tags["origin"] = c.GetOrigin()
	tags["_dd.origin"] = c.GetOrigin()
	return tags
}

func (c *CloudRun) getFunctionTags(tags map[string]string) map[string]string {
	functionTarget := os.Getenv(functionTargetEnvVar)
	functionSignatureType := os.Getenv(functionTypeEnvVar)

	if functionTarget != "" {
		tags[c.spanNamespace+"function_target"] = functionTarget
	}

	if functionSignatureType != "" {
		tags[c.spanNamespace+"function_signature_type"] = functionSignatureType
	}
	return tags
}

// GetOrigin returns the `origin` attribute type for the given
// cloud service.
func (c *CloudRun) GetOrigin() string {
	return "cloudrun"
}

// GetPrefix returns the prefix that we're prefixing all
// metrics with.
func (c *CloudRun) GetPrefix() string {
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
