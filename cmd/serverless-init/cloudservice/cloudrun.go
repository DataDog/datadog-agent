// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"os"
)

// CloudRun has helper functions for getting Google Cloud Run data
type CloudRun struct {
	cloudRunFunctionMode bool
}

// GetTags returns a map of gcp-related tags.
func (c *CloudRun) GetTags() map[string]string {
	return traceutil.GetCloudRunTags()
}

// GetOrigin returns the `origin` attribute type for the given cloud service.
func (c *CloudRun) GetOrigin() string {
	if c.cloudRunFunctionMode {
		_ = log.Warn("WE ARE IN CLOUD FUNCTION MODE")
		return "cloudfunction"
	}
	if isCloudFunction() {
		_ = log.Warn(fmt.Sprintf("WE ARE IN CLOUD FUNCTION MODE but %s", os.Environ()))
		return "cloudfunction"
	}
	_ = log.Warn(fmt.Sprintf("WE NOT IN CLOUD FUNCTION MODE", os.Environ()))
	return "cloudrun"
}

// GetPrefix returns the prefix that we're prefixing all metrics with.
func (c *CloudRun) GetPrefix() string {
	if c.cloudRunFunctionMode {
		_ = log.Warn("WE ARE IN CLOUD FUNCTION MODE")
		return "gcp.cloudfunction"
	}
	if isCloudFunction() {
		_ = log.Warn("WE ARE IN CLOUD FUNCTION MODE but i had to triple check")
		return "gcp.cloudfunction"
	}
	_ = log.Warn("Wow we ain't in cloud function mode")
	return "gcp.run"
}

// Init is empty for CloudRun
func (c *CloudRun) Init() error {
	return nil
}

func isCloudRunService() bool {
	_, exists := os.LookupEnv(traceutil.RunServiceNameEnvVar)
	return exists
}

func isCloudFunction() bool {
	_, cloudRunFunctionMode := os.LookupEnv(traceutil.FunctionTargetEnvVar)
	_ = log.Warn(fmt.Sprintf("cloud function mode SET TO: %B", cloudRunFunctionMode))
	return cloudRunFunctionMode
}
