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
				"job_name:example__job_name",
				"databricks_cluster_name:example___job_name",
				"databricks_cluster_id:cluster123",
				"cluster_id:cluster123",
				"cluster_name:example___job_name",
				"databricks_workspace:example_workspace",
				"workspace:example_workspace",
			},
		},
		{
			name: "with job, run ids",
			env: map[string]string{
				"DB_CLUSTER_NAME": "job-123-run-456",
			},
			wantTags: []string{
				"data_workload_monitoring_trial:true",
				"databricks_cluster_name:job-123-run-456",
				"cluster_name:job-123-run-456",
				"jobid:123",
				"runid:456",
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
				"job_name:example__job_name",
				"databricks_cluster_name:example___job_name",
				"databricks_cluster_id:cluster123",
				"cluster_id:cluster123",
				"cluster_name:example___job_name",
				"databricks_workspace:\"example_workspace\"",
				"workspace:example_workspace",
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
