// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"fmt"
	"os"
)

// CloudRunJobsOrigin origin tag value
const CloudRunJobsOrigin = "cloudrun"

const (
	cloudRunJobNameEnvVar     = "CLOUD_RUN_JOB"
	cloudRunExecutionEnvVar   = "CLOUD_RUN_EXECUTION"
	cloudRunTaskIndexEnvVar   = "CLOUD_RUN_TASK_INDEX"
	cloudRunTaskAttemptEnvVar = "CLOUD_RUN_TASK_ATTEMPT"
	cloudRunTaskCountEnvVar   = "CLOUD_RUN_TASK_COUNT"
)

const (
	cloudRunJobNamespace = "gcrj."
	jobNameTag           = "job_name"
	executionNameTag     = "execution_name"
	taskIndexTag         = "task_index"
	taskAttemptTag       = "task_attempt"
	taskCountTag         = "task_count"
	resourceNameTag      = "resource_name"
)

// CloudRunJobs has helper functions for getting Google Cloud Run data
type CloudRunJobs struct{}

// GetTags returns a map of gcp-related tags for Cloud Run Jobs.
func (c *CloudRunJobs) GetTags() map[string]string {
	tags := metadataHelperFunc(GetDefaultConfig(), false)
	tags["_dd.origin"] = CloudRunJobsOrigin

	jobNameVal := os.Getenv(cloudRunJobNameEnvVar)
	executionNameVal := os.Getenv(cloudRunExecutionEnvVar)
	taskIndexVal := os.Getenv(cloudRunTaskIndexEnvVar)
	taskAttemptVal := os.Getenv(cloudRunTaskAttemptEnvVar)
	taskCountVal := os.Getenv(cloudRunTaskCountEnvVar)

	if jobNameVal != "" {
		tags[cloudRunJobNamespace+jobNameTag] = jobNameVal
	}

	if executionNameVal != "" {
		tags[cloudRunJobNamespace+executionNameTag] = executionNameVal
	}

	if taskIndexVal != "" {
		tags[cloudRunJobNamespace+taskIndexTag] = taskIndexVal
	}

	if taskAttemptVal != "" {
		tags[cloudRunJobNamespace+taskAttemptTag] = taskAttemptVal
	}

	if taskCountVal != "" {
		tags[cloudRunJobNamespace+taskCountTag] = taskCountVal
	}

	tags[cloudRunJobNamespace+resourceNameTag] = fmt.Sprintf("projects/%s/locations/%s/jobs/%s", tags["project_id"], tags["location"], jobNameVal)
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

// GetStartMetricName returns the metric name for container start events
func (c *CloudRunJobs) GetStartMetricName() string {
	return fmt.Sprintf("%s.enhanced.start", c.GetPrefix())
}

func isCloudRunJob() bool {
	_, exists := os.LookupEnv(cloudRunJobNameEnvVar)
	return exists
}
