package djm

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/fleet/telemetry"
	"os"
	"testing"
)

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
"github.com/DataDog/datadog-agent/pkg/fleet/telemetry"
)

func TestSetupCommonEmrHostTags(t *testing.T) {

	originalExecuteCommand := executeCommandWithTimeout
	defer func() { executeCommandWithTimeout = originalExecuteCommand }() // Restore original after test

	executeCommandWithTimeout = func(command string, args ...string) ([]byte, error) {
		if command == "aws" && args[0] == "emr" && args[1] == "describe-cluster" {
			return []byte(`{"Cluster": {"Name": "TestCluster"}}`), nil
		}
		return nil, fmt.Errorf("unexpected command: %s", command)
	}

	tests := []struct {
		name     string
		env      map[string]string
		wantTags []string
	}{
		{
			name: "basic fields with formatting",
			env: map[string]string{
				"DB_DRIVER_IP":      "192.168.1.100",
				"DB_INSTANCE_TYPE":  "m4.xlarge",
				"DB_IS_JOB_CLUSTER": "true",
				"DD_JOB_NAME":       "example,'job,name",
				"DB_CLUSTER_NAME":   "example[,'job]name",
				"DB_CLUSTER_ID":     "cluster123",
			},
			wantTags: []string{
				"spark_host_ip:192.168.1.100",
				"databricks_instance_type:m4.xlarge",
				"databricks_is_job_cluster:true",
				"job_name:example__job_name",
				"databricks_cluster_name:example___job_name",
				"databricks_cluster_id:cluster123",
				"cluster_id:cluster123",
				"cluster_name:example___job_name",
			},
		},
		{
			name: "with job, run ids",
			env: map[string]string{
				"DB_CLUSTER_NAME": "job-123-run-456",
			},
			wantTags: []string{
				"databricks_cluster_name:job-123-run-456",
				"cluster_name:job-123-run-456",
				"jobid:123",
				"runid:456",
			},
		},
		{
			name:     "Missing env vars results in no tags",
			env:      map[string]string{},
			wantTags: []string{},
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

func TestSetupCommonEmrHostTags(t *testing.T) {
	mockReadFile := func(filename string) ([]byte, error) {
		if filename == "path/to/instance.json" {
			return []byte(`{"InstanceGroupID": "ig-123", "IsMaster": "true"}`), nil
		}
		if filename == "path/to/extraInstanceData.json" {
			return []byte(`{"JobFlowID": "j-456", "ReleaseLabel": "emr-6.3.0"}`), nil
		}
		return nil, errors.New("file not found")
	}

	mockExecuteCommand := func(command string, args ...string) ([]byte, error) {
		if command == "aws" && args[0] == "emr" && args[1] == "describe-cluster" {
			return []byte(`{"Cluster": {"Name": "TestCluster"}}`), nil
		}
		return nil, errors.New("command execution failed")
	}

	s := &common.Setup{} // Mock or initialize your setup object
	isMaster, clusterName, err := setupCommonEmrHostTags(s, mockReadFile, mockExecuteCommand)

	assert.NoError(t, err)
	assert.Equal(t, "true", isMaster)
	assert.Equal(t, "TestCluster", clusterName)
}
