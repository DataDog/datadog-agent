// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"fmt"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice/helper"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"os"
)

const (
	// Environment var needed for service
	revisionNameEnvVar      = "K_REVISION"
	ServiceNameEnvVar       = "K_SERVICE" // ServiceNameEnvVar is also used in the trace package
	configurationNameEnvVar = "K_CONFIGURATION"
	functionTypeEnvVar      = "FUNCTION_SIGNATURE_TYPE"
	functionTargetEnvVar    = "FUNCTION_TARGET" // exists as a cloudrunfunction env var for all runtimes except Go
)

const (
	// Span Tag with namespace specific for cloud run (gcr) and cloud run function (gcrfx)
	cloudRunService      = "gcr."
	cloudRunFunction     = "gcrfx."
	runRevisionName      = "gcr.revision_name"
	functionRevisionName = "gcrfx.revision_name"
	runServiceName       = "gcr.service_name"
	functionServiceName  = "gcrfx.service_name"
	runConfigName        = "gcr.configuration_name"
	functionConfigName   = "gcrfx.configuration_name"
)

var metadataHelperFunc = helper.GetMetaData

// CloudRun has helper functions for getting Google Cloud Run data
type CloudRun struct {
	spanNamespace string
}

// GetTags returns a map of gcp-related tags.
func (c *CloudRun) GetTags() map[string]string {
	tags := metadataHelperFunc(helper.GetDefaultConfig()).TagMap(c.spanNamespace)
	tags["origin"] = c.GetOrigin()
	tags["_dd.origin"] = c.GetOrigin()

	revisionName := os.Getenv(revisionNameEnvVar)
	serviceName := os.Getenv(ServiceNameEnvVar)
	configName := os.Getenv(configurationNameEnvVar)
	if revisionName != "" {
		tags["revision_name"] = revisionName
		if c.spanNamespace == cloudRunService {
			tags[runRevisionName] = revisionName
		} else {
			tags[functionRevisionName] = revisionName
		}
	}

	if serviceName != "" {
		tags["service_name"] = serviceName
		if c.spanNamespace == cloudRunService {
			tags[runServiceName] = serviceName
		} else {
			tags[functionServiceName] = serviceName
		}
	}

	if configName != "" {
		tags["configuration_name"] = configName
		if c.spanNamespace == cloudRunService {
			tags[runConfigName] = configName
		} else {
			tags[functionConfigName] = configName
		}
	}

	if c.spanNamespace == cloudRunFunction {
		return c.getFunctionTags(tags)
	}

	tags["_dd.gcr.resource_name"] = "projects/" + tags["project_id"] + "/locations/" + tags["location"] + "/services/" + serviceName
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

	tags["_dd.gcrfx.resource_name"] = "projects/" + tags["project_id"] + "/locations/" + tags["location"] + "/services/" + tags["service_name"] + "/functions/" + functionTarget
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
	log.Debug(fmt.Sprintf("cloud run namespace SET TO: %s", cloudRunFunction))
	return cloudRunFunctionMode
}
