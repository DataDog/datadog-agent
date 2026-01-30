// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/collector"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/exitcode"
	serverlessInitTrace "github.com/DataDog/datadog-agent/cmd/serverless-init/trace"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// CloudRunJobsOrigin origin tag value
const CloudRunJobsOrigin = "cloudrunjobs"

const (
	cloudRunJobNameEnvVar     = "CLOUD_RUN_JOB"
	cloudRunExecutionEnvVar   = "CLOUD_RUN_EXECUTION"
	cloudRunTaskIndexEnvVar   = "CLOUD_RUN_TASK_INDEX"
	cloudRunTaskAttemptEnvVar = "CLOUD_RUN_TASK_ATTEMPT"
	cloudRunTaskCountEnvVar   = "CLOUD_RUN_TASK_COUNT"
)

const (
	cloudRunJobNamespace = "gcrj."
	cloudRunJobsPrefix   = "gcp.run.job"
	// Low cardinality (include with metrics)
	jobNameTag      = "job_name"
	resourceNameTag = "resource_name"
	// High cardinality (avoid adding to metrics)
	executionNameTag = "execution_name"
	taskIndexTag     = "task_index"
	taskAttemptTag   = "task_attempt"
	taskCountTag     = "task_count" // not really high cardinality, but not necessary for metrics
)

// CloudRunJobs has helper functions for getting Google Cloud Run data
type CloudRunJobs struct {
	collector  *collector.Collector
	startTime  time.Time
	jobSpan    *pb.Span
	traceAgent TraceAgent
	spanTags   map[string]string // tags used for span creation (unified service tags + configured tags + cloud provider metadata)
}

// GetTags returns a map of gcp-related tags for Cloud Run Jobs.
func (c *CloudRunJobs) GetTags() map[string]string {
	tags := metadataHelperFunc(GetDefaultConfig(), false)
	tags["origin"] = CloudRunJobsOrigin
	tags["_dd.origin"] = CloudRunJobsOrigin

	jobNameVal := os.Getenv(cloudRunJobNameEnvVar)
	executionNameVal := os.Getenv(cloudRunExecutionEnvVar)
	taskIndexVal := os.Getenv(cloudRunTaskIndexEnvVar)
	taskAttemptVal := os.Getenv(cloudRunTaskAttemptEnvVar)
	taskCountVal := os.Getenv(cloudRunTaskCountEnvVar)

	if jobNameVal != "" {
		tags[cloudRunJobNamespace+jobNameTag] = jobNameVal
		tags[jobNameTag] = jobNameVal
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

// GetDefaultLogsSource returns the default logs source if `DD_SOURCE` is not set
func (c *CloudRunJobs) GetDefaultLogsSource() string {
	// Use the default log pipeline for Cloud Run.
	return CloudRunOrigin
}

func (c *CloudRunJobs) GetMetricPrefix() string {
	return cloudRunJobsPrefix
}

// GetOrigin returns the `origin` attribute type for the given cloud service.
func (c *CloudRunJobs) GetOrigin() string {
	return CloudRunJobsOrigin
}

// GetSource returns the metrics source
func (c *CloudRunJobs) GetSource() metrics.MetricSource {
	return metrics.MetricSourceGoogleCloudRunEnhanced
}

// Init records the start time for CloudRunJobs and initializes the job span
func (c *CloudRunJobs) Init(ctx *TracingContext) error {
	c.startTime = time.Now()
	if ctx != nil {
		c.traceAgent = ctx.TraceAgent
		c.spanTags = ctx.SpanTags
	}
	if pkgconfigsetup.Datadog().GetBool("apm_config.enabled") && pkgconfigsetup.Datadog().GetBool("serverless.trace_enabled") {
		c.initJobSpan()
		c.setSpanModifier()
	}
	return nil
}

// Shutdown submits the task duration and shutdown metrics for CloudRunJobs,
// and completes and submits the job span.
func (c *CloudRunJobs) Shutdown(metricAgent serverlessMetrics.ServerlessMetricAgent, runErr error) {
	durationMetricName := cloudRunJobsPrefix + ".enhanced.task.duration"
	duration := float64(time.Since(c.startTime).Milliseconds())
	metricAgent.AddMetric(durationMetricName, duration, c.GetSource(), metrics.DistributionType)

	shutdownMetricName := cloudRunJobsPrefix + ".enhanced.task.ended"
	exitCode := exitcode.From(runErr)
	succeededTag := "succeeded:true"
	if exitCode != 0 {
		succeededTag = "succeeded:false"
	}
	metricAgent.AddMetric(shutdownMetricName, 1.0, c.GetSource(), metrics.DistributionType, succeededTag)

	c.completeAndSubmitJobSpan(runErr)
}

func (c *CloudRunJobs) StartEnhancedMetrics(metricAgent *serverlessMetrics.ServerlessMetricAgent) {
	c.collector = startEnhancedMetrics(metricAgent, c.GetSource(), c.GetMetricPrefix())
}

// GetStartMetricName returns the metric name for container start events
func (c *CloudRunJobs) GetStartMetricName() string {
	return cloudRunJobsPrefix + ".enhanced.task.started"
}

// ShouldForceFlushAllOnForceFlushToSerializer is true for cloud run jobs.
func (c *CloudRunJobs) ShouldForceFlushAllOnForceFlushToSerializer() bool {
	return true
}

func isCloudRunJob() bool {
	_, exists := os.LookupEnv(cloudRunJobNameEnvVar)
	return exists
}

// initJobSpan creates and initializes the job span with Cloud Run Job metadata
func (c *CloudRunJobs) initJobSpan() {
	tags := c.spanTags
	jobNameVal := tags[jobNameTag]

	// Use DD_SERVICE for the service name, fallback to job name, then "gcp.run.job"
	serviceName := tags["service"]
	if serviceName == "" {
		serviceName = jobNameVal
	}
	if serviceName == "" {
		serviceName = "gcp.run.job"
	}
	log.Debugf("Cloud Run Job: using service name %q (tags[service]=%q, job_name=%q)", serviceName, tags["service"], jobNameVal)

	// Use job name for resource, fallback
	resourceName := jobNameVal
	if resourceName == "" {
		resourceName = "gcp.run.job"
	}

	c.jobSpan = serverlessInitTrace.InitSpan(
		serviceName,
		"gcp.run.job.task",
		resourceName,
		"", // TODO add custom 'job' span type (requires UI changes)
		c.startTime.UnixNano(),
		tags,
	)
}

// setSpanModifier sets up the span modifier to reparent user spans under the job span
func (c *CloudRunJobs) setSpanModifier() {
	if c.traceAgent == nil || c.jobSpan == nil {
		return
	}

	modifier := serverlessInitTrace.NewCloudRunJobsSpanModifier(c.jobSpan)
	if ta, ok := c.traceAgent.(serverlessInitTrace.SpanModifierSetter); ok {
		ta.SetSpanModifier(modifier)
	}
}

// completeAndSubmitJobSpan finalizes the span with duration and error status, then submits it
func (c *CloudRunJobs) completeAndSubmitJobSpan(runErr error) {
	if c.jobSpan == nil {
		return
	}

	c.jobSpan.Duration = time.Since(c.startTime).Nanoseconds()

	if runErr != nil {
		c.jobSpan.Error = 1
		c.jobSpan.Meta["error.msg"] = runErr.Error()
		exitCode := exitcode.From(runErr)
		c.jobSpan.Meta["exit_code"] = strconv.Itoa(exitCode)
	}

	serverlessInitTrace.SubmitSpan(c.jobSpan, CloudRunJobsOrigin, c.traceAgent)
}
