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
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/config"
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

func newTestSetup(t *testing.T) *common.Setup {
	t.Helper()
	span, _ := telemetry.StartSpanFromContext(context.Background(), "test")
	return &common.Setup{
		Span: span,
		Ctx:  context.Background(),
		Out:  &common.Output{},
	}
}

func TestSetupOpenLineage_Enabled(t *testing.T) {
	// Save and restore global state
	originalExecuteCommand := common.ExecuteCommandWithTimeout
	defer func() { common.ExecuteCommandWithTimeout = originalExecuteCommand }()
	origConfig := tracerConfigEmr
	defer func() { tracerConfigEmr = origConfig }()

	t.Setenv("DD_OPENLINEAGE_ENABLED", "true")

	var capturedCmd string
	var capturedArgs []string
	common.ExecuteCommandWithTimeout = func(_ *common.Setup, command string, args ...string) ([]byte, error) {
		capturedCmd = command
		capturedArgs = args
		return nil, nil
	}

	origJARDir := openLineageJARDir
	openLineageJARDir = t.TempDir()
	defer func() { openLineageJARDir = origJARDir }()
	// Place a scala-library JAR so variant detection works
	require.NoError(t, os.WriteFile(filepath.Join(openLineageJARDir, "scala-library-2.12.18.jar"), []byte{}, 0644))

	s := newTestSetup(t)
	setupOpenLineage(s)

	assert.Equal(t, config.BoolToPtr(true), tracerConfigEmr.DataJobsOpenLineageEnabled)
	assert.Equal(t, "curl", capturedCmd)
	expectedURL := "https://repo1.maven.org/maven2/io/openlineage/openlineage-spark_2.12/1.49.0/openlineage-spark_2.12-1.49.0.jar"
	assert.Contains(t, capturedArgs, expectedURL)
	assert.Contains(t, capturedArgs, filepath.Join(openLineageJARDir, "openlineage-spark_2.12-1.49.0.jar"))
}

func TestSetupOpenLineage_Disabled(t *testing.T) {
	origConfig := tracerConfigEmr
	defer func() { tracerConfigEmr = origConfig }()

	// DD_OPENLINEAGE_ENABLED not set
	t.Setenv("DD_OPENLINEAGE_ENABLED", "false")

	s := newTestSetup(t)
	setupOpenLineage(s)

	assert.Nil(t, tracerConfigEmr.DataJobsOpenLineageEnabled)
}

func TestSetupOpenLineage_JarAlreadyPresent(t *testing.T) {
	originalExecuteCommand := common.ExecuteCommandWithTimeout
	defer func() { common.ExecuteCommandWithTimeout = originalExecuteCommand }()
	origConfig := tracerConfigEmr
	defer func() { tracerConfigEmr = origConfig }()

	t.Setenv("DD_OPENLINEAGE_ENABLED", "true")

	commandCalled := false
	common.ExecuteCommandWithTimeout = func(_ *common.Setup, _ string, _ ...string) ([]byte, error) {
		commandCalled = true
		return nil, nil
	}

	// Create a temp dir with a fake existing JAR
	origJARDir := openLineageJARDir
	openLineageJARDir = t.TempDir()
	defer func() { openLineageJARDir = origJARDir }()
	require.NoError(t, os.WriteFile(filepath.Join(openLineageJARDir, "openlineage-spark_2.12-1.20.0.jar"), []byte{}, 0644))

	s := newTestSetup(t)
	setupOpenLineage(s)

	// Tracer flag should still be set
	assert.Equal(t, config.BoolToPtr(true), tracerConfigEmr.DataJobsOpenLineageEnabled)
	// But no download should have been attempted
	assert.False(t, commandCalled)
}

func TestSetupOpenLineage_CustomJarPath(t *testing.T) {
	originalExecuteCommand := common.ExecuteCommandWithTimeout
	defer func() { common.ExecuteCommandWithTimeout = originalExecuteCommand }()
	origConfig := tracerConfigEmr
	defer func() { tracerConfigEmr = origConfig }()

	t.Setenv("DD_OPENLINEAGE_ENABLED", "true")

	customJar := filepath.Join(t.TempDir(), "custom-ol.jar")
	require.NoError(t, os.WriteFile(customJar, []byte{}, 0644))
	t.Setenv("DD_OPENLINEAGE_JAR_PATH", customJar)

	var capturedCmd string
	var capturedArgs []string
	common.ExecuteCommandWithTimeout = func(_ *common.Setup, command string, args ...string) ([]byte, error) {
		capturedCmd = command
		capturedArgs = args
		return nil, nil
	}

	// Use a temp dir so the glob finds no existing JARs
	origJARDir := openLineageJARDir
	openLineageJARDir = t.TempDir()
	defer func() { openLineageJARDir = origJARDir }()

	s := newTestSetup(t)
	setupOpenLineage(s)

	assert.Equal(t, config.BoolToPtr(true), tracerConfigEmr.DataJobsOpenLineageEnabled)
	assert.Equal(t, "cp", capturedCmd)
	assert.Equal(t, customJar, capturedArgs[0])
}

func TestDetectScalaVariant(t *testing.T) {
	origJARDir := openLineageJARDir
	defer func() { openLineageJARDir = origJARDir }()

	s := newTestSetup(t)

	t.Run("detects 2.12", func(t *testing.T) {
		openLineageJARDir = t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(openLineageJARDir, "scala-library-2.12.18.jar"), []byte{}, 0644))
		assert.Equal(t, "_2.12", detectScalaVariant(s))
	})

	t.Run("detects 2.13", func(t *testing.T) {
		openLineageJARDir = t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(openLineageJARDir, "scala-library-2.13.12.jar"), []byte{}, 0644))
		assert.Equal(t, "_2.13", detectScalaVariant(s))
	})

	t.Run("falls back to 2.12 when no JAR found", func(t *testing.T) {
		openLineageJARDir = t.TempDir()
		assert.Equal(t, "_2.12", detectScalaVariant(s))
	})

	t.Run("env var overrides detection", func(t *testing.T) {
		openLineageJARDir = t.TempDir()
		// Place a 2.12 JAR, but override to 2.13 via env var
		require.NoError(t, os.WriteFile(filepath.Join(openLineageJARDir, "scala-library-2.12.18.jar"), []byte{}, 0644))
		t.Setenv("DD_OPENLINEAGE_SPARK_VARIANT", "_2.13")
		assert.Equal(t, "_2.13", detectScalaVariant(s))
	})
}
