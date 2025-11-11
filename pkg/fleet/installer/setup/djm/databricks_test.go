// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package djm contains data-jobs-monitoring installation logic
package djm

import (
	"context"
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
				"WORKSPACE_URL":        "https://dbc-12345678-a1b2.cloud.databricks.com/",
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
				"workspace_url:https://dbc-12345678-a1b2.cloud.databricks.com/",
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

func TestSetupGPUIntegration(t *testing.T) {
	tests := []struct {
		name                   string
		env                    map[string]string
		expectedEnableGPUM     *bool
		expectedSystemProbeGPU bool
	}{
		{
			name: "GPU monitoring enabled",
			env: map[string]string{
				"DD_GPU_ENABLED": "true",
			},
			expectedEnableGPUM:     config.BoolToPtr(true),
			expectedSystemProbeGPU: true,
		},
		{
			name: "GPU monitoring enabled with empty string value",
			env: map[string]string{
				"DD_GPU_ENABLED": "",
			},
			expectedEnableGPUM:     nil,
			expectedSystemProbeGPU: false,
		},
		{
			name:                   "GPU monitoring not set",
			env:                    map[string]string{},
			expectedEnableGPUM:     nil,
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

			if os.Getenv("DD_GPU_ENABLED") == "true" {
				setupGPUIntegration(s)
			}

			assert.Equal(t, tt.expectedEnableGPUM, s.Config.DatadogYAML.GPUCheck.Enabled)
		})
	}
}
