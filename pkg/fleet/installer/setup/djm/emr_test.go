// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package djm contains data-jobs-monitoring installation logic
package djm

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

//go:embed testdata/emrInstance.json
var instanceJSON string

//go:embed testdata/emrExtraInstanceData.json
var extraInstanceJSON string

//go:embed testdata/emrDescribeClusterResponse.json
var emrDescribeClusterResponse string

func TestSetupEmr(t *testing.T) {

	// Mock AWS emr describe command
	originalExecuteCommand := common.ExecuteCommandWithTimeout
	defer func() { common.ExecuteCommandWithTimeout = originalExecuteCommand }() // Restore original after test

	common.ExecuteCommandWithTimeout = func(s *common.Setup, command string, args ...string) (output []byte, err error) {
		span, _ := telemetry.StartSpanFromContext(s.Ctx, "setup.command")
		span.SetResourceName(command)
		defer func() { span.Finish(err) }()
		if command == "aws" && args[0] == "emr" && args[1] == "describe-cluster" {
			return []byte(emrDescribeClusterResponse), nil
		}
		return nil, fmt.Errorf("unexpected command: %s", command)
	}

	// Write info files in temp dir
	emrInfoPath = t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(emrInfoPath, "instance.json"), []byte(instanceJSON), 0644))

	require.NoError(t, os.WriteFile(filepath.Join(emrInfoPath, "extraInstanceData.json"), []byte(extraInstanceJSON), 0644))

	tests := []struct {
		name     string
		wantTags []string
	}{
		{
			name: "basic fields json",
			wantTags: []string{
				"data_workload_monitoring_trial:true",
				"instance_group_id:ig-123",
				"is_master_node:true",
				"job_flow_id:j-456",
				"cluster_id:j-456",
				"emr_version:emr-7.2.0",
				"cluster_name:TestCluster",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span, _ := telemetry.StartSpanFromContext(context.Background(), "test")
			s := &common.Setup{
				Span: span,
				Ctx:  context.Background(),
			}

			_, _, err := setupCommonEmrHostTags(s)
			assert.Nil(t, err)
			assert.ElementsMatch(t, tt.wantTags, s.Config.DatadogYAML.Tags)
		})
	}
}
