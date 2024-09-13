// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"os"
)

var cloudRunFunctionMode bool

// CloudRun has helper functions for getting Google Cloud Run data
type CloudRun struct{}

// GetTags returns a map of gcp-related tags.
func (c *CloudRun) GetTags() map[string]string {
	return traceutil.GetCloudRunTags()
}

// GetOrigin returns the `origin` attribute type for the given
// cloud service.
func (c *CloudRun) GetOrigin() string {
	if cloudRunFunctionMode {
		_ = log.Warn("WE ARE IN CLOUD FUNCTION MODE")
		return "cloudfunction"
	}
	_ = log.Warn("WE NOT IN CLOUD FUNCTION MODE")
	return "cloudrun"
}

// GetPrefix returns the prefix that we're prefixing all
// metrics with.
func (c *CloudRun) GetPrefix() string {
	if cloudRunFunctionMode {
		_ = log.Warn("WE ARE IN CLOUD FUNCTION MODE")
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
	_, cloudRunFunctionMode = os.LookupEnv(traceutil.FunctionTargetEnvVar)
	return exists
}
