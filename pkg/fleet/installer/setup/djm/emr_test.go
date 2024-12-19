// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package djm contains data-jobs-monitoring installation logic
package djm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/fleet/telemetry"
)

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

func TestSetupCommonEmrHostTags(t *testing.T) {

	// Mock AWS emr describe command
	originalExecuteCommand := executeCommandWithTimeout
	defer func() { executeCommandWithTimeout = originalExecuteCommand }() // Restore original after test

	executeCommandWithTimeout = func(command string, args ...string) ([]byte, error) {
		if command == "aws" && args[0] == "emr" && args[1] == "describe-cluster" {
			return []byte(`{"Cluster": {"Name": "TestCluster"}}`), nil
		}
		return nil, fmt.Errorf("unexpected command: %s", command)
	}

	// Write info files in temp dir
	emrInfoPath = t.TempDir()
	instanceJSON := `{"InstanceGroupID": "ig-123", "IsMaster": true}`
	extraInstanceJSON := `{"JobFlowID": "j-456", "ReleaseLabel": "emr-7.2.0"}`
	err := os.WriteFile(filepath.Join(emrInfoPath, "instance.json"), []byte(instanceJSON), 0644)

	if err != nil {
		t.Fatalf("failed to write instance.json: %v", err)
	}

	err = os.WriteFile(filepath.Join(emrInfoPath, "extraInstanceData.json"), []byte(extraInstanceJSON), 0644)
	if err != nil {
		t.Fatalf("failed to write extraInstanceData.json: %v", err)
	}

	tests := []struct {
		name     string
		env      map[string]string
		wantTags []string
	}{
		{
			name: "basic fields json",

			wantTags: []string{
				"instance_group_id:ig-123",
				"is_master_node:true",
				"job_flow_id:j-456",
				"cluster_id:j-456",
				"cluster_name:TestCluster",
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

			isMaster, clusterName, err := setupCommonEmrHostTags(s)
			fmt.Println(isMaster, clusterName, err)
			assert.Nil(t, err)
			assert.ElementsMatch(t, tt.wantTags, s.Config.DatadogYAML.Tags)
		})
	}
}
