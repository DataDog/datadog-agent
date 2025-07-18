// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
		"_dd.origin":          "cloudrun",
		"project_id":          "test_project",
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
	assert.Equal(t, "cloudrun", service.GetOrigin())
}

func TestCloudRunJobsGetPrefix(t *testing.T) {
	service := &CloudRunJobs{}
	assert.Equal(t, "gcp.run.job", service.GetPrefix())
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
