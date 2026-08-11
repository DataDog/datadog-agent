// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseIndex(t *testing.T) {
	cases := []struct {
		name    string
		choice  string
		n       int
		wantIdx int
		wantOK  bool
	}{
		{"first", "1", 3, 0, true},
		{"last", "3", 3, 2, true},
		{"zero", "0", 3, 0, false},
		{"too big", "4", 3, 0, false},
		{"not a number", "x", 3, 0, false},
		{"empty options", "1", 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idx, ok := parseIndex(tc.choice, tc.n)
			assert.Equal(t, tc.wantOK, ok)
			if ok {
				assert.Equal(t, tc.wantIdx, idx)
			}
		})
	}
}

func TestSummarizeStatus(t *testing.T) {
	def := TestDefinition{Agent: agentConfig{
		Installer: "helm-k8s", AgentVersion: "7.81", ClusterAgentVersion: "latest", Namespace: "datadog",
	}}

	assert.Equal(t, "not provisioned", summarizeStatus(envState{}, def))

	assert.Equal(t, "provisioned, agent not installed",
		summarizeStatus(envState{"kubernetesCluster": json.RawMessage(`{}`)}, def))

	agentRaw, err := json.Marshal(map[string]any{
		"linuxNodeAgent":    map[string]string{"namespace": "datadog", "installVersion": "7.81"},
		"linuxClusterAgent": map[string]string{"namespace": "datadog", "installVersion": "latest"},
	})
	require.NoError(t, err)
	status := summarizeStatus(envState{
		"kubernetesCluster": json.RawMessage(`{}`),
		"agent":             agentRaw,
	}, def)
	assert.Contains(t, status, "up to date")
}

func TestDiscoverEnvs(t *testing.T) {
	dir := t.TempDir()

	configPath := filepath.Join(t.TempDir(), "good.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("name: good\nprovisioner:\n  type: kind\n"), 0o644))

	goodState := envState{"kubernetesCluster": json.RawMessage(`{}`)}
	require.NoError(t, setSourcePath(goodState, configPath))
	require.NoError(t, writeStateFileAtomic(filepath.Join(dir, "good.state.json"), goodState))

	// No "_source" recorded — must be skipped, not error the whole scan.
	require.NoError(t, writeStateFileAtomic(filepath.Join(dir, "orphan.state.json"), envState{"kubernetesCluster": json.RawMessage(`{}`)}))

	// "_source" points at a YAML that no longer exists — same treatment.
	missingState := envState{}
	require.NoError(t, setSourcePath(missingState, filepath.Join(t.TempDir(), "gone.yaml")))
	require.NoError(t, writeStateFileAtomic(filepath.Join(dir, "missing-source.state.json"), missingState))

	// "_source" is valid JSON but not a string (e.g. an array) — same
	// treatment. (A syntactically invalid RawMessage can't be used here:
	// json.RawMessage.MarshalJSON validates its bytes via compact(), so
	// writeStateFileAtomic itself would fail to produce the fixture.)
	require.NoError(t, writeStateFileAtomic(filepath.Join(dir, "unparseable-source.state.json"), envState{
		"_source": json.RawMessage(`["not", "a", "string"]`),
	}))

	envs, err := discoverEnvs(dir)
	require.NoError(t, err)
	require.Len(t, envs, 1)
	assert.Equal(t, "good", envs[0].Def.Name)
	assert.Equal(t, configPath, envs[0].ConfigPath)
	assert.Equal(t, "provisioned, agent not installed", envs[0].Status)
}
