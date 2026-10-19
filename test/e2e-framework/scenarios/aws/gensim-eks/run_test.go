// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gensimeks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func envMapFromValues(t *testing.T, parsed map[string]any) map[string]string {
	t.Helper()
	datadog, ok := parsed["datadog"].(map[string]any)
	require.True(t, ok)
	envItems, ok := datadog["env"].([]any)
	require.True(t, ok)
	result := make(map[string]string, len(envItems))
	for _, raw := range envItems {
		entry, ok := raw.(map[string]any)
		require.True(t, ok)
		name, _ := entry["name"].(string)
		val, _ := entry["value"].(string)
		result[name] = val
	}
	return result
}

func TestRenderAgentValuesSemanticMatchWithVMRecorder(t *testing.T) {
	rendered, err := renderAgentValuesWithConfig(agentValuesConfig{
		AgentImage:    "docker.io/datadog/agent-dev:my-branch-recorder-full",
		Mode:          "record-parquet",
		FlushInterval: "1m",
		Retention:     "90m",
		ClusterName:   "gensim-bench",
	})
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(rendered), &parsed))

	env := envMapFromValues(t, parsed)
	require.Equal(t, "true", env["DD_ANOMALY_DETECTION_RECORDING_ENABLED"])
	require.Equal(t, "/tmp/observer-metrics", env["DD_ANOMALY_DETECTION_RECORDING_OUTPUT_DIR"])
	require.Equal(t, "1m", env["DD_ANOMALY_DETECTION_RECORDING_FLUSH_INTERVAL"])
	require.Equal(t, "90m", env["DD_ANOMALY_DETECTION_RECORDING_RETENTION"])
	require.Equal(t, "true", env["DD_ANOMALY_DETECTION_RECORDING_ONLY"])
	require.Equal(t, "false", env["DD_ANOMALY_DETECTION_REPORTING_EVENTS_ENABLED"])

	for key := range env {
		require.False(t, strings.HasPrefix(key, "DD_OBSERVER_"), "legacy DD_OBSERVER_* env var rendered: %s", key)
	}

	datadog := parsed["datadog"].(map[string]any)
	require.Equal(t, "gensim-secrets", datadog["apiKeyExistingSecret"])
	require.Equal(t, "gensim-secrets", datadog["appKeyExistingSecret"])
	require.Equal(t, "gensim-bench", datadog["clusterName"])

	agents := parsed["agents"].(map[string]any)
	image := agents["image"].(map[string]any)
	require.Equal(t, "docker.io/datadog/agent-dev", image["repository"])
	require.Equal(t, "my-branch-recorder-full", image["tag"])
	require.Equal(t, "Always", image["pullPolicy"])

	customCfg := agents["customAgentConfig"].(map[string]any)
	require.NotContains(t, customCfg, "observer")
	adCfg := customCfg["anomaly_detection"].(map[string]any)
	recCfg := adCfg["recording"].(map[string]any)
	require.Equal(t, true, recCfg["enabled"])
	require.Equal(t, "/tmp/observer-metrics", recCfg["output_dir"])
	require.Equal(t, "1m", recCfg["flush_interval"])
	require.Equal(t, "90m", recCfg["retention"])
}

func TestRenderAgentValuesModesAndValidation(t *testing.T) {
	liveValues, err := renderAgentValues("docker.io/datadog/agent-dev:my-tag", "live-anomaly-detection")
	require.NoError(t, err)
	var parsedLive map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(liveValues), &parsedLive))
	liveEnv := envMapFromValues(t, parsedLive)
	require.Equal(t, "false", liveEnv["DD_ANOMALY_DETECTION_RECORDING_ENABLED"])
	require.Equal(t, "true", liveEnv["DD_ANOMALY_DETECTION_REPORTING_EVENTS_ENABLED"])

	bothValues, err := renderAgentValues("docker.io/datadog/agent-dev:my-recorder-tag", "live-and-record")
	require.NoError(t, err)
	var parsedBoth map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(bothValues), &parsedBoth))
	bothEnv := envMapFromValues(t, parsedBoth)
	require.Equal(t, "true", bothEnv["DD_ANOMALY_DETECTION_RECORDING_ENABLED"])
	require.Equal(t, "true", bothEnv["DD_ANOMALY_DETECTION_REPORTING_EVENTS_ENABLED"])
	require.Equal(t, "false", bothEnv["DD_ANOMALY_DETECTION_RECORDING_ONLY"])

	invalidCases := []struct {
		name string
		cfg  agentValuesConfig
		err  string
	}{
		{
			name: "unsupported mode",
			cfg:  agentValuesConfig{AgentImage: "docker.io/datadog/agent-dev:tag", Mode: "unknown"},
			err:  "unsupported mode",
		},
		{
			name: "missing tag",
			cfg:  agentValuesConfig{AgentImage: "docker.io/datadog/agent-dev", Mode: "record-parquet"},
			err:  "invalid image reference",
		},
		{
			name: "latest tag rejected for recording",
			cfg:  agentValuesConfig{AgentImage: "docker.io/datadog/agent-dev:latest", Mode: "record-parquet"},
			err:  "not allowed",
		},
		{
			name: "non-recorder release repo rejected",
			cfg:  agentValuesConfig{AgentImage: "datadog/agent:7.55.0", Mode: "record-parquet"},
			err:  "anomaly_detection_recorder",
		},
		{
			name: "retention not greater than flush",
			cfg:  agentValuesConfig{AgentImage: "docker.io/datadog/agent-dev:tag", Mode: "record-parquet", FlushInterval: "5m", Retention: "5m"},
			err:  "retention (5m) must exceed flushInterval (5m)",
		},
		{
			name: "recordingOnly incompatible with live mode",
			cfg:  agentValuesConfig{AgentImage: "docker.io/datadog/agent-dev:tag", Mode: "live-and-record", RecordingOnly: "true"},
			err:  "recordingOnly=true is only supported with mode=record-parquet",
		},
	}

	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			_, renderErr := renderAgentValuesWithConfig(tc.cfg)
			require.ErrorContains(t, renderErr, tc.err)
		})
	}
}

func TestGroupEpisodeConfigsMultipleScenariosSameDirectory(t *testing.T) {
	repoRoot := t.TempDir()
	epDir := filepath.Join(repoRoot, "postmortems", "authcore_pgbouncer")
	require.NoError(t, os.MkdirAll(filepath.Join(epDir, "episodes"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(epDir, "chart", "templates"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(epDir, "play-episode.sh"), []byte("#!/bin/sh\necho ok\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(epDir, "chart", "Chart.yaml"), []byte("apiVersion: v2\nname: test\nversion: 0.1.0\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(epDir, "episodes", "pool-saturation.yaml"), []byte("name: pool-saturation\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(epDir, "episodes", "conn-spike.yaml"), []byte("name: conn-spike\n"), 0o644))

	specs, err := groupEpisodeConfigs(repoRoot, "authcore_pgbouncer:pool-saturation,authcore_pgbouncer:conn-spike")
	require.NoError(t, err)
	require.Len(t, specs, 1)
	require.Equal(t, "gensim-ep-authcore-pgbouncer", specs[0].ConfigMapName)
	require.Equal(t, "ep-authcore-pgbouncer", specs[0].VolumeName)
	require.Equal(t, "/episodes/authcore_pgbouncer", specs[0].MountPath)
	require.Equal(t, []string{"pool-saturation", "conn-spike"}, specs[0].Scenarios)
	require.Contains(t, specs[0].Data, "play-episode.sh")
	require.Contains(t, specs[0].Data, "pool-saturation.yaml")
	require.Contains(t, specs[0].Data, "conn-spike.yaml")
	require.NotEmpty(t, specs[0].BinaryData["chart.tar.gz"])
}
