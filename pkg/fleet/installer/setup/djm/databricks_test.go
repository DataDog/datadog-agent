// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package djm contains data-jobs-monitoring installation logic
package djm

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/config"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

func TestSetupCommonHostTags(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		wantTags []string
	}{
		{
			name: "basic fields with formatting",
			env: map[string]string{
				"DB_DRIVER_IP":         "192.168.1.100",
				"DB_INSTANCE_TYPE":     "m4.xlarge",
				"DB_IS_JOB_CLUSTER":    "true",
				"DD_JOB_NAME":          "example,'job,name",
				"DB_CLUSTER_NAME":      "example[,'job]name",
				"DB_CLUSTER_ID":        "cluster123",
				"DATABRICKS_WORKSPACE": "example_workspace",
			},
			wantTags: []string{
				"data_workload_monitoring_trial:true",
				"spark_host_ip:192.168.1.100",
				"databricks_instance_type:m4.xlarge",
				"databricks_is_job_cluster:true",
				"job_name:example_job_name",
				"databricks_cluster_name:example_job_name",
				"databricks_cluster_id:cluster123",
				"cluster_id:cluster123",
				"cluster_name:example_job_name",
				"databricks_workspace:example_workspace",
				"workspace:example_workspace",
				"dd.internal.resource:databricks_cluster:cluster123",
			},
		},
		{
			name: "with job, run ids but not job cluster",
			env: map[string]string{
				"DB_CLUSTER_NAME": "job-123-run-456",
			},
			wantTags: []string{
				"data_workload_monitoring_trial:true",
				"databricks_cluster_name:job-123-run-456",
				"cluster_name:job-123-run-456",
				"jobid:123",
				"runid:456",
				"dd.internal.resource:databricks_job:123",
			},
		},
		{
			name: "with job, run ids and is job cluster",
			env: map[string]string{
				"DB_CLUSTER_NAME":   "job-123-run-456",
				"DB_IS_JOB_CLUSTER": "TRUE",
			},
			wantTags: []string{
				"data_workload_monitoring_trial:true",
				"databricks_cluster_name:job-123-run-456",
				"databricks_is_job_cluster:TRUE",
				"cluster_name:job-123-run-456",
				"jobid:123",
				"runid:456",
				"dd.internal.resource:databricks_job:123",
				"dd.internal.resource:databricks_cluster:123",
			},
		},
		{
			name: "Missing env vars results in no tags",
			env:  map[string]string{},
			wantTags: []string{
				"data_workload_monitoring_trial:true",
			},
		},
		{
			name: "workspace name with quotes",
			env: map[string]string{
				"DB_DRIVER_IP":         "192.168.1.100",
				"DB_INSTANCE_TYPE":     "m4.xlarge",
				"DB_IS_JOB_CLUSTER":    "true",
				"DD_JOB_NAME":          "example,'job,name",
				"DB_CLUSTER_NAME":      "example[,'job]name",
				"DB_CLUSTER_ID":        "cluster123",
				"DATABRICKS_WORKSPACE": "\"example_workspace\"",
			},
			wantTags: []string{
				"data_workload_monitoring_trial:true",
				"spark_host_ip:192.168.1.100",
				"databricks_instance_type:m4.xlarge",
				"databricks_is_job_cluster:true",
				"job_name:example_job_name",
				"databricks_cluster_name:example_job_name",
				"databricks_cluster_id:cluster123",
				"cluster_id:cluster123",
				"cluster_name:example_job_name",
				"databricks_workspace:\"example_workspace\"",
				"workspace:example_workspace",
				"dd.internal.resource:databricks_cluster:cluster123",
			},
		},
		{
			name: "workspace name forbidden chars",
			env: map[string]string{
				"DB_DRIVER_IP":         "192.168.1.100",
				"DB_INSTANCE_TYPE":     "m4.xlarge",
				"DB_IS_JOB_CLUSTER":    "true",
				"DD_JOB_NAME":          "example,'job,name",
				"DB_CLUSTER_NAME":      "example[,'job]name",
				"DB_CLUSTER_ID":        "cluster123",
				"DATABRICKS_WORKSPACE": "Example Workspace",
			},
			wantTags: []string{
				"data_workload_monitoring_trial:true",
				"spark_host_ip:192.168.1.100",
				"databricks_instance_type:m4.xlarge",
				"databricks_is_job_cluster:true",
				"job_name:example_job_name",
				"databricks_cluster_name:example_job_name",
				"databricks_cluster_id:cluster123",
				"cluster_id:cluster123",
				"cluster_name:example_job_name",
				"databricks_workspace:Example Workspace",
				"workspace:example_workspace",
				"dd.internal.resource:databricks_cluster:cluster123",
			},
		},
		{
			name: "job cluster with cluster ID and DB_IS_JOB_CLUSTER=TRUE",
			env: map[string]string{
				"DB_CLUSTER_ID":     "cluster-67890",
				"DB_CLUSTER_NAME":   "job-999-run-888",
				"DB_IS_JOB_CLUSTER": "TRUE",
			},
			wantTags: []string{
				"data_workload_monitoring_trial:true",
				"databricks_cluster_name:job-999-run-888",
				"databricks_cluster_id:cluster-67890",
				"databricks_is_job_cluster:TRUE",
				"cluster_id:cluster-67890",
				"cluster_name:job-999-run-888",
				"jobid:999",
				"runid:888",
				"dd.internal.resource:databricks_job:999",
				"dd.internal.resource:databricks_cluster:999",
			},
		},
		{
			name: "job pattern but not a job cluster (DB_IS_JOB_CLUSTER not TRUE)",
			env: map[string]string{
				"DB_CLUSTER_ID":   "cluster-67890",
				"DB_CLUSTER_NAME": "job-999-run-888",
			},
			wantTags: []string{
				"data_workload_monitoring_trial:true",
				"databricks_cluster_name:job-999-run-888",
				"databricks_cluster_id:cluster-67890",
				"cluster_id:cluster-67890",
				"cluster_name:job-999-run-888",
				"jobid:999",
				"runid:888",
				"dd.internal.resource:databricks_job:999",
				"dd.internal.resource:databricks_cluster:cluster-67890",
			},
		},
		{
			name: "only cluster ID env var",
			env: map[string]string{
				"DB_CLUSTER_ID": "cluster-only-12345",
			},
			wantTags: []string{
				"data_workload_monitoring_trial:true",
				"databricks_cluster_id:cluster-only-12345",
				"cluster_id:cluster-only-12345",
				"dd.internal.resource:databricks_cluster:cluster-only-12345",
			},
		},
		{
			name: "DB_IS_JOB_CLUSTER=TRUE but no job pattern in cluster name",
			env: map[string]string{
				"DB_CLUSTER_ID":     "regular-cluster",
				"DB_CLUSTER_NAME":   "my-regular-cluster",
				"DB_IS_JOB_CLUSTER": "TRUE",
			},
			wantTags: []string{
				"data_workload_monitoring_trial:true",
				"databricks_cluster_name:my-regular-cluster",
				"databricks_cluster_id:regular-cluster",
				"databricks_is_job_cluster:TRUE",
				"cluster_id:regular-cluster",
				"cluster_name:my-regular-cluster",
				"dd.internal.resource:databricks_cluster:regular-cluster",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range tt.env {
				require.NoError(t, os.Setenv(k, v))
			}
			span, _ := telemetry.StartSpanFromContext(context.Background(), "test")
			s := &common.Setup{Span: span}

			setupCommonHostTags(s)

			assert.ElementsMatch(t, tt.wantTags, s.Config.DatadogYAML.Tags)
		})
	}
}

func TestGetJobAndRunIDs(t *testing.T) {
	tests := []struct {
		name          string
		env           map[string]string
		expectedJobID string
		expectedRunID string
		expectedOk    bool
	}{
		{
			name: "Valid job and run ID",
			env: map[string]string{
				"DB_CLUSTER_NAME": "job-605777310657626-run-1020337925419295-databricks_cost_job_cluster",
			},
			expectedJobID: "605777310657626",
			expectedRunID: "1020337925419295",
			expectedOk:    true,
		},
		{
			name: "Invalid cluster name",
			env: map[string]string{
				"DB_CLUSTER_NAME": "invalid-cluster-name",
			},
			expectedJobID: "",
			expectedRunID: "",
			expectedOk:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range tt.env {
				require.NoError(t, os.Setenv(k, v))
			}

			jobID, runID, ok := getJobAndRunIDs()

			assert.Equal(t, tt.expectedJobID, jobID)
			assert.Equal(t, tt.expectedRunID, runID)
			assert.Equal(t, tt.expectedOk, ok)
		})
	}
}

func TestLoadLogProcessingRules(t *testing.T) {
	tests := []struct {
		name           string
		envValue       string
		expectedRules  []config.LogProcessingRule
		expectErrorLog bool
	}{
		{
			name:     "Valid rules",
			envValue: `[{"type":"exclude_at_match","name":"exclude_health_check","pattern":"GET /health"}]`,
			expectedRules: []config.LogProcessingRule{
				{
					Type:    "exclude_at_match",
					Name:    "exclude_health_check",
					Pattern: "GET /health",
				},
			},
			expectErrorLog: false,
		},
		{
			name:     "Valid rules with single quotes",
			envValue: `[{'type':'exclude_at_match','name':'exclude_health_check','pattern':'GET /health'}]`,
			expectedRules: []config.LogProcessingRule{
				{
					Type:    "exclude_at_match",
					Name:    "exclude_health_check",
					Pattern: "GET /health",
				},
			},
			expectErrorLog: false,
		},
		{
			name:           "Empty input",
			envValue:       ``,
			expectedRules:  nil,
			expectErrorLog: false,
		},
		{
			name:           "Invalid JSON",
			envValue:       `[{"type":"exclude_at_match","name":"bad_rule",]`,
			expectedRules:  nil,
			expectErrorLog: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			require.NoError(t, os.Setenv("DD_LOGS_CONFIG_PROCESSING_RULES", tt.envValue))

			output := &common.Output{}

			span, _ := telemetry.StartSpanFromContext(context.Background(), "test")
			s := &common.Setup{
				Span: span,
				Out:  output,
				Config: config.Config{
					DatadogYAML: config.DatadogConfig{},
				},
			}

			loadLogProcessingRules(s)

			assert.Equal(t, tt.expectedRules, s.Config.DatadogYAML.LogsConfig.ProcessingRules)
		})
	}
}

func TestAddTagsToConfig(t *testing.T) {
	tests := []struct {
		name     string
		tags     map[string]string
		wantTags []string
	}{
		{
			name: "Basic tags",
			tags: map[string]string{
				"environment": "production",
				"team":        "data-platform",
				"cost-center": "123456",
			},
			wantTags: []string{
				"environment:production",
				"team:data-platform",
				"cost-center:123456",
			},
		},
		{
			name: "Tags with colons in keys",
			tags: map[string]string{
				"databricks:env":  "production",
				"databricks:team": "data-platform",
			},
			wantTags: []string{
				"databricks:env:production",
				"databricks:team:data-platform",
			},
		},
		{
			name: "Tags with colons in values",
			tags: map[string]string{
				"environment": "prod:east",
				"region":      "us:east-1",
			},
			wantTags: []string{
				"environment:prod:east",
				"region:us:east-1",
			},
		},
		{
			name:     "Empty tags",
			tags:     map[string]string{},
			wantTags: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span, _ := telemetry.StartSpanFromContext(context.Background(), "test")
			s := &common.Setup{
				Span: span,
				Config: config.Config{
					DatadogYAML: config.DatadogConfig{},
				},
			}

			addTagsToConfig(s, tt.tags)

			assert.ElementsMatch(t, tt.wantTags, s.Config.DatadogYAML.Tags)
		})
	}
}

func TestFetchDatabricksCustomTags(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{
			name: "missing token and host",
			env:  map[string]string{},
		},
		{
			name: "missing token",
			env: map[string]string{
				"DATABRICKS_HOST": "https://example.com",
			},
		},
		{
			name: "missing host",
			env: map[string]string{
				"DATABRICKS_TOKEN": "abc123",
			},
		},
		{
			name: "token and host present but no cluster ID",
			env: map[string]string{
				"DATABRICKS_TOKEN": "abc123",
				"DATABRICKS_HOST":  "https://example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range tt.env {
				require.NoError(t, os.Setenv(k, v))
			}

			span, _ := telemetry.StartSpanFromContext(context.Background(), "test")
			output := &common.Output{}
			s := &common.Setup{
				Span: span,
				Out:  output,
				Config: config.Config{
					DatadogYAML: config.DatadogConfig{},
				},
			}

			// This should not panic when token or host are missing
			fetchDatabricksCustomTags(s)

			assert.Empty(t, s.Config.DatadogYAML.Tags)
		})
	}
}

// TestFetchDatabricksCustomTagsWithMock tests the fetchDatabricksCustomTags function
// by mocking the HTTP client and API responses
func TestFetchDatabricksCustomTagsWithMock(t *testing.T) {
	// Save original functions to restore after test
	originalFetchClusterTags := fetchClusterTagsFunc
	originalFetchJobTags := fetchJobTagsFunc

	defer func() {
		// Restore original functions after test
		fetchClusterTagsFunc = originalFetchClusterTags
		fetchJobTagsFunc = originalFetchJobTags
	}()

	tests := []struct {
		name                string
		env                 map[string]string
		mockClusterTags     map[string]string
		mockClusterSparkVer string
		mockJobTags         map[string]string
		wantTags            []string
	}{
		{
			name: "successful fetch of cluster tags",
			env: map[string]string{
				"DATABRICKS_TOKEN": "abc123",
				"DATABRICKS_HOST":  "https://example.com",
				"DB_CLUSTER_ID":    "cluster123",
			},
			mockClusterTags: map[string]string{
				"environment": "production",
				"team":        "data-platform",
			},
			mockClusterSparkVer: "",
			mockJobTags:         nil,
			wantTags: []string{
				"environment:production",
				"team:data-platform",
			},
		},
		{
			name: "successful fetch of cluster tags with spark version",
			env: map[string]string{
				"DATABRICKS_TOKEN": "abc123",
				"DATABRICKS_HOST":  "https://example.com",
				"DB_CLUSTER_ID":    "cluster123",
			},
			mockClusterTags: map[string]string{
				"environment": "production",
				"team":        "data-platform",
			},
			mockClusterSparkVer: "15.4.x-scala2.12",
			mockJobTags:         nil,
			wantTags: []string{
				"environment:production",
				"team:data-platform",
				"runtime:15.4.x-scala2.12",
			},
		},
		{
			name: "successful fetch of job tags",
			env: map[string]string{
				"DATABRICKS_TOKEN": "abc123",
				"DATABRICKS_HOST":  "https://example.com",
				"DB_CLUSTER_NAME":  "job-123-run-456",
			},
			mockClusterTags:     nil,
			mockClusterSparkVer: "",
			mockJobTags: map[string]string{
				"cost-center": "data-eng",
				"project":     "analytics",
			},
			wantTags: []string{
				"cost-center:data-eng",
				"project:analytics",
			},
		},
		{
			name: "successful fetch of both cluster and job tags with spark version",
			env: map[string]string{
				"DATABRICKS_TOKEN": "abc123",
				"DATABRICKS_HOST":  "https://example.com",
				"DB_CLUSTER_ID":    "cluster123",
				"DB_CLUSTER_NAME":  "job-123-run-456",
			},
			mockClusterTags: map[string]string{
				"environment": "production",
			},
			mockClusterSparkVer: "15.4.x-scala2.12",
			mockJobTags: map[string]string{
				"project": "analytics",
			},
			wantTags: []string{
				"environment:production",
				"project:analytics",
				"runtime:15.4.x-scala2.12",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range tt.env {
				require.NoError(t, os.Setenv(k, v))
			}

			// Mock the fetchClusterTags function
			fetchClusterTagsFunc = func(_ *http.Client, _, _, _ string, _ *common.Setup) (map[string]string, error) {
				tags := make(map[string]string)
				// Copy the mock cluster tags
				for k, v := range tt.mockClusterTags {
					tags[k] = v
				}
				// Add spark version as runtime tag if provided
				if tt.mockClusterSparkVer != "" {
					tags["runtime"] = tt.mockClusterSparkVer
				}
				return tags, nil
			}

			// Mock the fetchJobTags function
			fetchJobTagsFunc = func(_ *http.Client, _, _, _ string, _ *common.Setup) (map[string]string, error) {
				return tt.mockJobTags, nil
			}

			span, ctx := telemetry.StartSpanFromContext(context.Background(), "test")
			output := &common.Output{}
			s := &common.Setup{
				Ctx:  ctx,
				Span: span,
				Out:  output,
				Config: config.Config{
					DatadogYAML: config.DatadogConfig{},
				},
			}

			fetchDatabricksCustomTags(s)

			for _, tag := range tt.wantTags {
				assert.Contains(t, s.Config.DatadogYAML.Tags, tag)
			}
		})
	}
}

func TestSetupGPUIntegration(t *testing.T) {
	tests := []struct {
		name                   string
		env                    map[string]string
		expectedCollectGPUTags bool
		expectedEnableNVML     bool
		expectedSystemProbeGPU bool
	}{
		{
			name: "GPU monitoring enabled",
			env: map[string]string{
				"DD_GPU_MONITORING_ENABLED": "true",
			},
			expectedCollectGPUTags: true,
			expectedEnableNVML:     true,
			expectedSystemProbeGPU: true,
		},
		{
			name: "GPU monitoring enabled with empty string value",
			env: map[string]string{
				"DD_GPU_MONITORING_ENABLED": "",
			},
			expectedCollectGPUTags: false,
			expectedEnableNVML:     false,
			expectedSystemProbeGPU: false,
		},
		{
			name:                   "GPU monitoring not set",
			env:                    map[string]string{},
			expectedCollectGPUTags: false,
			expectedEnableNVML:     false,
			expectedSystemProbeGPU: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range tt.env {
				require.NoError(t, os.Setenv(k, v))
			}

			span, _ := telemetry.StartSpanFromContext(context.Background(), "test")
			output := &common.Output{}
			s := &common.Setup{
				Span: span,
				Out:  output,
				Config: config.Config{
					DatadogYAML: config.DatadogConfig{},
				},
			}

			if os.Getenv("DD_GPU_MONITORING_ENABLED") == "true" {
				setupGPUIntegration(s)
			}

			assert.Equal(t, tt.expectedCollectGPUTags, s.Config.DatadogYAML.CollectGPUTags)
			assert.Equal(t, tt.expectedEnableNVML, s.Config.DatadogYAML.EnableNVMLDetection)

			// Check system-probe configuration
			if tt.expectedSystemProbeGPU {
				assert.NotNil(t, s.Config.SystemProbeYAML)
				assert.Equal(t, tt.expectedSystemProbeGPU, s.Config.SystemProbeYAML.GPUMonitoringConfig.Enabled)
			} else {
				if s.Config.SystemProbeYAML != nil {
					assert.Equal(t, tt.expectedSystemProbeGPU, s.Config.SystemProbeYAML.GPUMonitoringConfig.Enabled)
				}
			}

		})
	}
}
