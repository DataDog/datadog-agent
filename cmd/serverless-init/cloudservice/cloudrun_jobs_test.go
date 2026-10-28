// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	demultiplexer "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/def"
	demultiplexerimpl "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/impl"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	serverlessdef "github.com/DataDog/datadog-agent/comp/metadata/serverlessinit/def"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-mock"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// mockTraceProcessor is a mock implementation of the TraceAgent interface for testing
type mockTraceProcessor struct {
	processCalled bool
	lastPayload   *api.Payload
}

func (m *mockTraceProcessor) Process(p *api.Payload) {
	m.processCalled = true
	m.lastPayload = p
}

func (m *mockTraceProcessor) Flush() {
	// no-op for tests
}

func (m *mockTraceProcessor) Stop() {
	// no-op for tests
}

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Cloud Run Jobs are not supported on Windows")
	}
}

func TestGetCloudRunJobsTagsWithEnvironmentVariables(t *testing.T) {
	skipOnWindows(t)
	service := &CloudRunJobs{}

	metadataHelperFunc = func(*GCPConfig, CloudRunType) map[string]string {
		return map[string]string{
			"container_id": "test_container",
			"location":     "test_region",
			"project_id":   "test_project",
		}
	}

	t.Setenv("CLOUD_RUN_JOB", "test_job")
	t.Setenv("CLOUD_RUN_EXECUTION", "test_execution")
	t.Setenv("CLOUD_RUN_TASK_INDEX", "0")
	t.Setenv("CLOUD_RUN_TASK_ATTEMPT", "1")
	t.Setenv("CLOUD_RUN_TASK_COUNT", "5")

	tags := service.GetTags()

	assert.Equal(t, map[string]string{
		"container_id":        "test_container",
		"location":            "test_region",
		"origin":              "cloudrunjobs",
		"_dd.origin":          "cloudrunjobs",
		"project_id":          "test_project",
		"job_name":            "test_job",
		"gcrj.job_name":       "test_job",
		"gcrj.execution_name": "test_execution",
		"gcrj.task_index":     "0",
		"gcrj.task_attempt":   "1",
		"gcrj.task_count":     "5",
		"gcrj.resource_name":  "projects/test_project/locations/test_region/jobs/test_job",
	}, tags)
}

func TestCloudRunJobsGetMetadata(t *testing.T) {
	skipOnWindows(t)
	service := &CloudRunJobs{}

	metadataHelperFunc = func(*GCPConfig, CloudRunType) map[string]string {
		return map[string]string{
			"container_id": "test_container",
			"location":     "test_region",
			"project_id":   "test_project",
		}
	}

	t.Setenv("CLOUD_RUN_JOB", "test_job")
	t.Setenv("CLOUD_RUN_EXECUTION", "test_execution")

	service.GetTags()
	metadata := service.GetMetadata()

	assert.Equal(t, serverlessdef.CloudServiceMetadata{
		WorkloadType:     workloadTypeCloudRunJob,
		ResourceID:       "//run.googleapis.com/projects/test_project/locations/test_region/executions/test_execution",
		ParentResourceID: "//run.googleapis.com/projects/test_project/locations/test_region/jobs/test_job",
		ResourceName:     "test_job",
		Region:           "test_region",
		GCPProjectID:     "test_project",
	}, metadata)
	assert.True(t, service.CanCollectMetadata())
}

// TestCloudRunJobsGetMetadataMissingExecution pins that a missing
// CLOUD_RUN_EXECUTION must not degrade the resource_id to the job ID.
func TestCloudRunJobsGetMetadataMissingExecution(t *testing.T) {
	skipOnWindows(t)
	service := &CloudRunJobs{}

	metadataHelperFunc = func(*GCPConfig, CloudRunType) map[string]string {
		return map[string]string{
			"location":   "test_region",
			"project_id": "test_project",
		}
	}

	t.Setenv("CLOUD_RUN_JOB", "test_job")
	os.Unsetenv("CLOUD_RUN_EXECUTION")

	service.GetTags()
	metadata := service.GetMetadata()

	assert.Empty(t, metadata.ResourceID)
	assert.Equal(t, "//run.googleapis.com/projects/test_project/locations/test_region/jobs/test_job", metadata.ParentResourceID)
}

func TestCloudRunJobExecutionIdentity(t *testing.T) {
	saved := metadataHelperFunc
	t.Cleanup(func() { metadataHelperFunc = saved })
	instance := "first-instance"
	metadataHelperFunc = func(*GCPConfig, CloudRunType) map[string]string {
		return map[string]string{projectID: "project", location: "region", containerID: instance}
	}
	t.Setenv(cloudRunJobNameEnvVar, "unknown")
	t.Setenv(cloudRunExecutionEnvVar, "execution")
	t.Setenv(cloudRunTaskIndexEnvVar, "0")
	t.Setenv(cloudRunTaskAttemptEnvVar, "0")
	t.Setenv(cloudRunTaskCountEnvVar, "2")
	firstTask := &CloudRunJobs{}
	firstTask.GetTags()
	first := firstTask.GetMetadata()
	require.Equal(t, "//run.googleapis.com/projects/project/locations/region/executions/execution", first.ResourceID)
	require.Equal(t, "//run.googleapis.com/projects/project/locations/region/jobs/unknown", first.ParentResourceID)
	t.Setenv(cloudRunTaskIndexEnvVar, "1")
	t.Setenv(cloudRunTaskAttemptEnvVar, "2")
	instance = "next-instance"
	service := &CloudRunJobs{}
	service.GetTags()
	assert.Equal(t, first, service.GetMetadata())
	t.Setenv(cloudRunExecutionEnvVar, "next-execution")
	next := service.GetMetadata()
	assert.Equal(t, "//run.googleapis.com/projects/project/locations/region/executions/next-execution", next.ResourceID)
	assert.NotEqual(t, first.ResourceID, next.ResourceID)
	assert.Equal(t, first.ParentResourceID, next.ParentResourceID)
	assert.Equal(t, first.ResourceName, next.ResourceName)
}

func TestCloudRunJobsMetadataSharesTagLookup(t *testing.T) {
	for _, available := range []bool{false, true} {
		t.Run(fmt.Sprintf("metadata=%t", available), func(t *testing.T) {
			t.Setenv(cloudRunJobNameEnvVar, "job")
			t.Setenv(cloudRunExecutionEnvVar, "execution")
			t.Setenv(cloudRunTaskIndexEnvVar, "1")
			t.Setenv(cloudRunTaskAttemptEnvVar, "2")
			t.Setenv(cloudRunTaskCountEnvVar, "3")
			service := &CloudRunJobs{}
			gcpMetadata := map[string]string{projectID: "", location: "", containerID: ""}
			if available {
				gcpMetadata = map[string]string{projectID: "project", location: "region", containerID: "instance"}
			}
			saved := metadataHelperFunc
			t.Cleanup(func() { metadataHelperFunc = saved })
			calls := 0
			metadataHelperFunc = func(_ *GCPConfig, kind CloudRunType) map[string]string {
				calls++
				assert.Equal(t, CloudRunJob, kind)
				return gcpMetadata
			}

			assert.Empty(t, service.GetMetadata().ResourceID, "metadata before GetTags has no GCP values")
			assert.Zero(t, calls, "GetMetadata must not fetch GCP metadata")

			first := service.GetTags()
			first[projectID] = "mutated-by-caller"
			second := service.GetTags()
			assert.Equal(t, gcpMetadata[projectID], second[projectID])
			assert.NotContains(t, gcpMetadata, "origin", "tag enrichment must not mutate cached metadata")
			assert.Equal(t, CloudRunJobsOrigin, second["origin"])
			assert.Equal(t, "job", second[cloudRunJobTagPrefix+jobNameTag])
			assert.Equal(t, "execution", second[cloudRunJobTagPrefix+executionNameTag])
			assert.Equal(t, "1", second[cloudRunJobTagPrefix+taskIndexTag])
			assert.Equal(t, "2", second[cloudRunJobTagPrefix+taskAttemptTag])
			assert.Equal(t, "3", second[cloudRunJobTagPrefix+taskCountTag])
			if available {
				assert.Equal(t, "projects/project/locations/region/jobs/job", second[cloudRunJobTagPrefix+resourceNameTag])
				assert.NotEmpty(t, service.GetMetadata().ResourceID)
			} else {
				assert.NotContains(t, second, cloudRunJobTagPrefix+resourceNameTag)
				assert.Equal(t, "unknown", service.GetEnhancedMetricTags(second).Base[projectID])
				assert.Empty(t, service.GetMetadata().ResourceID)
			}
			assert.Equal(t, 1, calls, "metadata and telemetry share the cached lookup, including failures")
		})
	}
}

func TestCloudRunJobsGetEnhancedMetricTags(t *testing.T) {
	skipOnWindows(t)
	service := &CloudRunJobs{}
	tags := map[string]string{
		"job_name":     "test-job",
		"location":     "us-central1",
		"origin":       "cloudrunjobs",
		"project_id":   "test-project",
		"container_id": "abc123",
	}
	result := service.GetEnhancedMetricTags(tags)

	assert.Equal(t, map[string]string{
		"job_name":   "test-job",
		"location":   "us-central1",
		"origin":     "cloudrunjobs",
		"project_id": "test-project",
	}, result.Base)

	assert.Equal(t, map[string]string{
		"job_name":   "test-job",
		"location":   "us-central1",
		"origin":     "cloudrunjobs",
		"project_id": "test-project",
		"instance":   "abc123",
	}, result.Usage)
}

func TestCloudRunJobsGetUsageMetricSuffix(t *testing.T) {
	skipOnWindows(t)
	service := &CloudRunJobs{}
	assert.Equal(t, "instance", service.GetUsageMetricSuffix())
}

func TestCloudRunJobsGetOrigin(t *testing.T) {
	skipOnWindows(t)
	service := &CloudRunJobs{}
	assert.Equal(t, "cloudrunjobs", service.GetOrigin())
}

func TestCloudRunJobsInit(t *testing.T) {
	skipOnWindows(t)
	service := &CloudRunJobs{}
	assert.NoError(t, service.Init(nil))
}

func TestIsCloudRunJob(t *testing.T) {
	skipOnWindows(t)
	// Test when environment variable is set
	t.Setenv("CLOUD_RUN_JOB", "test-job")
	assert.True(t, isCloudRunJob())
}

func TestIsCloudRunJobWhenNotSet(t *testing.T) {
	skipOnWindows(t)
	// This test runs in a clean environment where CLOUD_RUN_JOB is not set
	assert.False(t, isCloudRunJob())
}

func TestCloudRunJobsShutdownAddsExitCodeTag(t *testing.T) {
	skipOnWindows(t)
	demux := createDemultiplexer(t)
	agent := &serverlessMetrics.ServerlessMetricAgent{Demux: demux}

	jobs := &CloudRunJobs{startTime: time.Now().Add(-time.Second)}
	shutdownMetricName := "gcp.run.job.enhanced.task.ended"

	cmd := exec.Command("bash", "-c", "exit 1")
	err := cmd.Run()
	require.Error(t, err)
	jobs.Shutdown(agent, true, err)

	generatedMetrics, timedMetrics := demux.WaitForSamples(100 * time.Millisecond)
	assert.Empty(t, timedMetrics)
	assert.Len(t, generatedMetrics, 2)

	foundShutdown := false
	for _, sample := range generatedMetrics {
		if sample.Name == shutdownMetricName {
			require.Contains(t, sample.Tags, "succeeded:false")
			foundShutdown = true
		}
	}
	assert.True(t, foundShutdown, "shutdown metric not emitted")
}

func TestCloudRunJobsShutdownExitCodeZeroOnSuccess(t *testing.T) {
	skipOnWindows(t)
	demux := createDemultiplexer(t)
	agent := &serverlessMetrics.ServerlessMetricAgent{Demux: demux}

	jobs := &CloudRunJobs{startTime: time.Now().Add(-time.Second)}
	shutdownMetricName := "gcp.run.job.enhanced.task.ended"

	jobs.Shutdown(agent, true, nil)

	generatedMetrics, _ := demux.WaitForSamples(100 * time.Millisecond)

	foundShutdown := false
	for _, sample := range generatedMetrics {
		if sample.Name == shutdownMetricName {
			require.Contains(t, sample.Tags, "succeeded:true")
			foundShutdown = true
		}
	}
	assert.True(t, foundShutdown, "shutdown metric not emitted")
}

func TestCloudRunJobsSpanCreation(t *testing.T) {
	skipOnWindows(t)
	t.Setenv("CLOUD_RUN_JOB", "my-test-job")

	// spanTags simulates the combined tags passed from main.go
	spanTags := map[string]string{
		"project_id": "test-project",
		"location":   "us-central1",
		"job_name":   "my-test-job",
		"origin":     "cloudrunjobs",
		"_dd.origin": "cloudrunjobs",
	}

	jobs := &CloudRunJobs{}
	jobs.Init(&TracingContext{SpanTags: spanTags})

	// Verify span was created
	assert.NotNil(t, jobs.jobSpan)
	if jobs.jobSpan != nil {
		// Service should fallback to job name since DD_SERVICE is not in tags
		assert.Equal(t, "my-test-job", jobs.jobSpan.Service)
		assert.Equal(t, "gcp.run.job.task", jobs.jobSpan.Name)
		assert.Equal(t, "my-test-job", jobs.jobSpan.Resource)
		assert.NotZero(t, jobs.jobSpan.TraceID)
		assert.NotZero(t, jobs.jobSpan.SpanID)
		assert.Equal(t, uint64(0), jobs.jobSpan.ParentID)
		assert.NotNil(t, jobs.jobSpan.Meta)
		assert.Equal(t, "cloudrunjobs", jobs.jobSpan.Meta["origin"])
		// Verify _dd.measured=1 is set for stats computation
		assert.Equal(t, float64(1), jobs.jobSpan.Metrics["_dd.measured"])
	}
}

func TestCloudRunJobsSpanServiceNameFallbackToJobName(t *testing.T) {
	skipOnWindows(t)
	t.Setenv("CLOUD_RUN_JOB", "my-job-name")

	// spanTags without "service" tag - should fallback to job name
	spanTags := map[string]string{
		"project_id": "test-project",
		"location":   "us-central1",
		"job_name":   "my-job-name",
		"origin":     "cloudrunjobs",
	}

	jobs := &CloudRunJobs{}
	jobs.Init(&TracingContext{SpanTags: spanTags})

	// Verify span falls back to job name for service name
	require.NotNil(t, jobs.jobSpan)
	assert.Equal(t, "my-job-name", jobs.jobSpan.Service)
	assert.Equal(t, "my-job-name", jobs.jobSpan.Resource)
}

func TestCloudRunJobsSpanServiceNameFromDDService(t *testing.T) {
	skipOnWindows(t)
	// spanTags with "service" tag set (from DD_SERVICE env var)
	spanTags := map[string]string{
		"project_id": "test-project",
		"location":   "us-central1",
		"job_name":   "my-job-name",
		"service":    "my-custom-service", // Set via DD_SERVICE in main.go
		"origin":     "cloudrunjobs",
	}

	jobs := &CloudRunJobs{}
	jobs.Init(&TracingContext{SpanTags: spanTags})

	// Verify span uses the DD_SERVICE value
	require.NotNil(t, jobs.jobSpan)
	assert.Equal(t, "my-custom-service", jobs.jobSpan.Service)
	// Resource should still use the job name
	assert.Equal(t, "my-job-name", jobs.jobSpan.Resource)
}

func TestCloudRunJobsSpanServiceNameFallbackToDefault(t *testing.T) {
	skipOnWindows(t)
	// spanTags without "service" tag and without "job_name" - should fallback to default
	spanTags := map[string]string{
		"project_id": "test-project",
		"location":   "us-central1",
		"origin":     "cloudrunjobs",
		// No "service" tag, no "job_name" tag
	}

	jobs := &CloudRunJobs{}
	jobs.Init(&TracingContext{SpanTags: spanTags})

	// Verify span falls back to default "gcp.run.job"
	require.NotNil(t, jobs.jobSpan)
	assert.Equal(t, "gcp.run.job", jobs.jobSpan.Service)
	assert.Equal(t, "gcp.run.job", jobs.jobSpan.Resource)
}

func TestCloudRunJobsCompleteAndSubmitJobSpanWithError(t *testing.T) {
	skipOnWindows(t)
	t.Setenv("CLOUD_RUN_JOB", "test-job")

	spanTags := map[string]string{
		"project_id": "test-project",
		"location":   "us-central1",
		"job_name":   "test-job",
		"origin":     "cloudrunjobs",
	}

	mockAgent := &mockTraceProcessor{}
	jobs := &CloudRunJobs{}
	jobs.Init(&TracingContext{TraceAgent: mockAgent, SpanTags: spanTags})

	// Simulate an error
	testErr := errors.New("task failed")
	jobs.Shutdown(nil, true, testErr)

	// Verify the span was submitted
	assert.True(t, mockAgent.processCalled)
	assert.NotNil(t, mockAgent.lastPayload)

	// Verify span has error information
	require.NotNil(t, jobs.jobSpan)
	assert.Equal(t, int32(1), jobs.jobSpan.Error)
	assert.Equal(t, "task failed", jobs.jobSpan.Meta["error.msg"])
	assert.Equal(t, "1", jobs.jobSpan.Meta["exit_code"])
	assert.NotZero(t, jobs.jobSpan.Duration)
}

func TestCloudRunJobsCompleteAndSubmitJobSpanSuccess(t *testing.T) {
	skipOnWindows(t)
	t.Setenv("CLOUD_RUN_JOB", "success-job")

	spanTags := map[string]string{
		"project_id": "test-project",
		"location":   "us-central1",
		"job_name":   "success-job",
		"origin":     "cloudrunjobs",
	}

	mockAgent := &mockTraceProcessor{}
	jobs := &CloudRunJobs{}
	jobs.Init(&TracingContext{TraceAgent: mockAgent, SpanTags: spanTags})

	// Simulate success (no error)
	jobs.Shutdown(nil, true, nil)

	// Verify the span was submitted
	assert.True(t, mockAgent.processCalled)
	assert.NotNil(t, mockAgent.lastPayload)

	// Verify span has no error
	require.NotNil(t, jobs.jobSpan)
	assert.Equal(t, int32(0), jobs.jobSpan.Error)
	assert.NotContains(t, jobs.jobSpan.Meta, "error.msg")
	assert.NotZero(t, jobs.jobSpan.Duration)
}

func TestCloudRunJobsCompleteAndSubmitJobSpanWithNilSpan(t *testing.T) {
	skipOnWindows(t)
	mockAgent := &mockTraceProcessor{}
	jobs := &CloudRunJobs{}
	// Don't call Init, so jobSpan remains nil

	// Should not panic
	jobs.Shutdown(nil, true, nil)

	// Should not submit anything
	assert.False(t, mockAgent.processCalled)
}

func createDemultiplexer(t *testing.T) demultiplexer.FakeSamplerMock {
	return fxutil.Test[demultiplexer.FakeSamplerMock](t,
		fx.Provide(func() log.Component { return logmock.New(t) }),
		logscompression.MockModule(),
		metricscompression.MockModule(),
		demultiplexerimpl.FakeSamplerMockModule(),
		hostnameimpl.MockModule(),
	)
}
