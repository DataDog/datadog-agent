// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-mock"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestGetCloudRunJobsTagsWithEnvironmentVariables(t *testing.T) {
	service := &CloudRunJobs{}

	metadataHelperFunc = func(*GCPConfig, bool) map[string]string {
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

func TestCloudRunJobsGetOrigin(t *testing.T) {
	service := &CloudRunJobs{}
	assert.Equal(t, "cloudrunjobs", service.GetOrigin())
}

func TestCloudRunJobsInit(t *testing.T) {
	service := &CloudRunJobs{}
	assert.NoError(t, service.Init())
}

func TestIsCloudRunJob(t *testing.T) {
	// Test when environment variable is set
	t.Setenv("CLOUD_RUN_JOB", "test-job")
	assert.True(t, isCloudRunJob())
}

func TestIsCloudRunJobWhenNotSet(t *testing.T) {
	// This test runs in a clean environment where CLOUD_RUN_JOB is not set
	assert.False(t, isCloudRunJob())
}

func TestCloudRunJobsShutdownAddsExitCodeTag(t *testing.T) {
	demux := createDemultiplexer(t)
	agent := serverlessMetrics.ServerlessMetricAgent{Demux: demux}

	jobs := &CloudRunJobs{startTime: time.Now().Add(-time.Second)}
	shutdownMetricName := fmt.Sprintf("%s.enhanced.task.ended", cloudRunJobsPrefix)

	cmd := exec.Command("bash", "-c", "exit 1")
	err := cmd.Run()
	require.Error(t, err)
	jobs.Shutdown(agent, err)

	generatedMetrics, timedMetrics := demux.WaitForSamples(100 * time.Millisecond)
	assert.Empty(t, timedMetrics)
	assert.Len(t, generatedMetrics, 2)

	foundShutdown := false
	for _, sample := range generatedMetrics {
		if sample.Name == shutdownMetricName {
			require.Contains(t, sample.Tags, "exit_code:1")
			foundShutdown = true
		}
	}
	assert.True(t, foundShutdown, "shutdown metric not emitted")
}

func TestCloudRunJobsShutdownExitCodeZeroOnSuccess(t *testing.T) {
	demux := createDemultiplexer(t)
	agent := serverlessMetrics.ServerlessMetricAgent{Demux: demux}

	jobs := &CloudRunJobs{startTime: time.Now().Add(-time.Second)}
	shutdownMetricName := fmt.Sprintf("%s.enhanced.task.ended", cloudRunJobsPrefix)

	jobs.Shutdown(agent, nil)

	generatedMetrics, _ := demux.WaitForSamples(100 * time.Millisecond)

	foundShutdown := false
	for _, sample := range generatedMetrics {
		if sample.Name == shutdownMetricName {
			require.Contains(t, sample.Tags, "exit_code:0")
			foundShutdown = true
		}
	}
	assert.True(t, foundShutdown, "shutdown metric not emitted")
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
