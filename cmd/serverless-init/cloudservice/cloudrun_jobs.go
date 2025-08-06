// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"fmt"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/metric"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
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
	cloudRunJobsPrefix   = "gcp.run.job"
)

// CloudRunJobs has helper functions for getting Google Cloud Run data
type CloudRunJobs struct {
	startTime time.Time
}

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

// GetSource returns the metrics source
func (c *CloudRunJobs) GetSource() metrics.MetricSource {
	return metrics.MetricSourceGoogleCloudRunEnhanced
}

// Init records the start time for CloudRunJobs
func (c *CloudRunJobs) Init() error {
	c.startTime = time.Now()
	return nil
}

// Shutdown submits the task duration metric for CloudRunJobs
func (c *CloudRunJobs) Shutdown(metricAgent serverlessMetrics.ServerlessMetricAgent) {
	metricName := fmt.Sprintf("%s.enhanced.task.duration", cloudRunJobsPrefix)
	duration := float64(time.Since(c.startTime).Milliseconds())
	metric.Add(metricName, duration, c.GetSource(), metricAgent)
}

// GetStartMetricName returns the metric name for container start events
func (c *CloudRunJobs) GetStartMetricName() string {
	return fmt.Sprintf("%s.enhanced.task.started", cloudRunJobsPrefix)
}

// GetShutdownMetricName returns the metric name for container shutdown events
func (c *CloudRunJobs) GetShutdownMetricName() string {
	return fmt.Sprintf("%s.enhanced.task.ended", cloudRunJobsPrefix)
}

// ShouldForceFlushAllOnForceFlushToSerializer is true for cloud run jobs.
func (c *CloudRunJobs) ShouldForceFlushAllOnForceFlushToSerializer() bool {
	return true
}

func isCloudRunJob() bool {
	_, exists := os.LookupEnv(cloudRunJobNameEnvVar)
	return exists
}
