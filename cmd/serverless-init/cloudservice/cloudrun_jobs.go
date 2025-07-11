// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"os"
)

const CloudRunJobsOrigin = "cloudrun"

const (
	cloudRunJobNameEnvVar = "CLOUD_RUN_JOB"
)

type CloudRunJobs struct{}

// GetTags returns a map of gcp-related tags for Cloud Run Jobs.
func (c *CloudRunJobs) GetTags() map[string]string {
	tags := metadataHelperFunc(GetDefaultConfig(), false)
	// TODO
	return tags
}

// GetOrigin returns the `origin` attribute type for the given cloud service.
func (c *CloudRunJobs) GetOrigin() string {
	return CloudRunJobsOrigin
}

// GetPrefix returns the prefix that we're prefixing all metrics with.
func (c *CloudRunJobs) GetPrefix() string {
	return "gcp.run.job"
}

// Init is empty for CloudRunJobs
func (c *CloudRunJobs) Init() error {
	return nil
}

func isCloudRunJob() bool {
	_, exists := os.LookupEnv(cloudRunJobNameEnvVar)
	return exists
}
