// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gpu

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/config"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

func TestCTracerVersion(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		expected string
	}{
		{name: "unset uses latest major", env: "", expected: "0"},
		{name: "full version gets revision", env: "0.40.0", expected: "0.40.0-1"},
		{name: "version with revision kept", env: "0.40.0-1", expected: "0.40.0-1"},
		{name: "minor tag kept", env: "0.40", expected: "0.40"},
		{name: "major tag kept", env: "1", expected: "1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envCTracerVersion, tt.env)
			assert.Equal(t, tt.expected, cTracerVersion())
		})
	}
}

func TestSetupGPUCheck(t *testing.T) {
	span, _ := telemetry.StartSpanFromContext(context.Background(), "test")
	s := &common.Setup{Span: span, Out: &common.Output{}}

	setupGPUCheck(s)

	assert.Equal(t, config.BoolToPtr(true), s.Config.DatadogYAML.GPUCheck.Enabled)
	assert.True(t, s.DelayedAgentRestartConfig.Scheduled)
	assert.Equal(t, gpuRestartDelay, s.DelayedAgentRestartConfig.Delay)
	assert.Equal(t, gpuRestartLogFile, s.DelayedAgentRestartConfig.LogFile)
}

func TestAgentVersion(t *testing.T) {
	tests := []struct {
		name     string
		env      *env.Env
		expected string
	}{
		{name: "no env uses latest Agent 7", env: nil, expected: "7"},
		{name: "unset uses latest Agent 7", env: &env.Env{}, expected: "7"},
		{name: "minor version pin", env: &env.Env{AgentMinorVersion: "86.0"}, expected: "7.86.0-1"},
		{name: "minor version with revision", env: &env.Env{AgentMinorVersion: "86.0-1"}, expected: "7.86.0-1"},
		{name: "major and minor", env: &env.Env{AgentMajorVersion: "7", AgentMinorVersion: "86"}, expected: "7.86"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, agentVersion(&common.Setup{Env: tt.env}))
		})
	}
}

func TestSetupDatabricksHostTags(t *testing.T) {
	tests := []struct {
		name          string
		env           map[string]string
		wantTags      []string
		wantExtraTags []string
	}{
		{
			name: "driver with all variables",
			env: map[string]string{
				"DB_IS_DRIVER":               "TRUE",
				"DB_CLUSTER_ID":              "cluster123",
				"DB_CLUSTER_NAME":            "my [gpu] cluster",
				"DATABRICKS_WORKSPACE":       "example_workspace",
				"WORKSPACE_URL":              "https://dbc-1234.cloud.databricks.com/",
				"DB_INSTANCE_TYPE":           "g5.xlarge",
				"DB_IS_JOB_CLUSTER":          "TRUE",
				"DATABRICKS_RUNTIME_VERSION": "15.4",
				"DD_TAGS":                    "team:gpu,env:prod",
				"DD_EXTRA_TAGS":              "foo:bar baz:qux",
			},
			wantTags: []string{
				"databricks_cluster_id:cluster123",
				"cluster_id:cluster123",
				"databricks_cluster_name:my_gpu_cluster",
				"cluster_name:my_gpu_cluster",
				"databricks_workspace:example_workspace",
				"workspace_url:https://dbc-1234.cloud.databricks.com/",
				"databricks_instance_type:g5.xlarge",
				"databricks_is_job_cluster:TRUE",
				"databricks_runtime:15.4",
				"spark_node:driver",
				"team:gpu",
				"env:prod",
			},
			wantExtraTags: []string{"foo:bar", "baz:qux"},
		},
		{
			name:     "worker without variables",
			env:      map[string]string{},
			wantTags: []string{"spark_node:worker"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range []string{"DB_IS_DRIVER", "DB_CLUSTER_ID", "DB_CLUSTER_NAME", "DATABRICKS_WORKSPACE", "WORKSPACE_URL", "DB_INSTANCE_TYPE", "DB_IS_JOB_CLUSTER", "DATABRICKS_RUNTIME_VERSION", "DD_TAGS", "DD_EXTRA_TAGS"} {
				t.Setenv(k, "")
				require.NoError(t, os.Unsetenv(k))
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			s := &common.Setup{}

			setupDatabricksHostTags(s)

			assert.Equal(t, tt.wantTags, s.Config.DatadogYAML.Tags)
			assert.Equal(t, tt.wantExtraTags, s.Config.DatadogYAML.ExtraTags)
		})
	}
}

func TestSetupTracerEnv(t *testing.T) {
	newSetup := func() *common.Setup {
		span, _ := telemetry.StartSpanFromContext(context.Background(), "test")
		return &common.Setup{Span: span, Out: &common.Output{}}
	}

	t.Run("writes the tracer env and keeps existing lines", func(t *testing.T) {
		t.Setenv("DD_SERVICE", "")
		t.Setenv("DD_ENV", "")
		path := filepath.Join(t.TempDir(), "spark-env.sh")
		require.NoError(t, os.WriteFile(path, []byte("export FOO=bar"), 0644))

		require.NoError(t, setupTracerEnv(newSetup(), path))

		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "export FOO=bar\n"+
			sparkEnvBlockStart+"\n"+
			"export DD_INJECT_NATIVE='always'\n"+
			"export DD_GPU_ENABLED='true'\n"+
			sparkEnvBlockEnd+"\n", string(content))
	})

	t.Run("passes DD_SERVICE and DD_ENV through", func(t *testing.T) {
		t.Setenv("DD_SERVICE", "it's-a-service")
		t.Setenv("DD_ENV", "prod")
		path := filepath.Join(t.TempDir(), "spark-env.sh")

		require.NoError(t, setupTracerEnv(newSetup(), path))

		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(content), `export DD_SERVICE='it'\''s-a-service'`+"\n")
		assert.Contains(t, string(content), "export DD_ENV='prod'\n")
	})

	t.Run("rerun replaces the previous block", func(t *testing.T) {
		t.Setenv("DD_ENV", "")
		path := filepath.Join(t.TempDir(), "spark-env.sh")
		require.NoError(t, os.WriteFile(path, []byte("export FOO=bar\n"), 0644))

		t.Setenv("DD_SERVICE", "first")
		require.NoError(t, setupTracerEnv(newSetup(), path))
		t.Setenv("DD_SERVICE", "second")
		require.NoError(t, setupTracerEnv(newSetup(), path))

		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, 1, strings.Count(string(content), sparkEnvBlockStart))
		assert.NotContains(t, string(content), "first")
		assert.Contains(t, string(content), "export DD_SERVICE='second'\n")
		assert.True(t, strings.HasPrefix(string(content), "export FOO=bar\n"+sparkEnvBlockStart))
	})

	t.Run("skips when the Spark conf directory is missing", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing", "spark-env.sh")

		require.NoError(t, setupTracerEnv(newSetup(), path))

		_, err := os.Stat(path)
		assert.True(t, os.IsNotExist(err))
	})
}

func TestSetupTracerConfig(t *testing.T) {
	newSetup := func() *common.Setup {
		span, _ := telemetry.StartSpanFromContext(context.Background(), "test")
		return &common.Setup{Span: span, Out: &common.Output{}}
	}

	t.Run("default", func(t *testing.T) {
		t.Setenv("DD_SERVICE", "")
		t.Setenv("DD_ENV", "")
		s := newSetup()

		setupTracerConfig(s)

		require.NotNil(t, s.Config.ApplicationMonitoringYAML)
		assert.Equal(t, config.APMConfigurationDefault{
			InjectNative: "always",
			GPUEnabled:   config.BoolToPtr(true),
		}, s.Config.ApplicationMonitoringYAML.Default)
	})

	t.Run("passes DD_SERVICE and DD_ENV through", func(t *testing.T) {
		t.Setenv("DD_SERVICE", "my-service")
		t.Setenv("DD_ENV", "prod")
		s := newSetup()

		setupTracerConfig(s)

		assert.Equal(t, "my-service", s.Config.ApplicationMonitoringYAML.Default.Service)
		assert.Equal(t, "prod", s.Config.ApplicationMonitoringYAML.Default.Env)
	})
}
