// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadStateFileMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")

	st, err := readStateFile(path)
	require.NoError(t, err)
	assert.Empty(t, st)
}

func TestWriteReadStateFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	want := envState{
		"kubernetesCluster": json.RawMessage(`{"foo":"bar"}`),
	}

	require.NoError(t, writeStateFileAtomic(path, want))

	got, err := readStateFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, `{"foo":"bar"}`, string(got["kubernetesCluster"]))
}

func TestStagesCompleted(t *testing.T) {
	cases := []struct {
		name                           string
		st                             envState
		wantProvisioned, wantInstalled bool
	}{
		{"empty", envState{}, false, false},
		{"provisioned only", envState{"kubernetesCluster": json.RawMessage(`{}`)}, true, false},
		{"provisioned and installed", envState{
			"kubernetesCluster": json.RawMessage(`{}`),
			"agent":             json.RawMessage(`{}`),
		}, true, true},
		{"provisioned via a different provisioner's resource key", envState{"vmHost": json.RawMessage(`{}`)}, true, false},
		{"metadata alone is not provisioned", envState{"_source": json.RawMessage(`"/x.yaml"`)}, false, false},
		{"metadata alongside agent is installed but not provisioned", envState{
			"_source": json.RawMessage(`"/x.yaml"`),
			"agent":   json.RawMessage(`{}`),
		}, false, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provisioned, installed := stagesCompleted(tc.st)
			assert.Equal(t, tc.wantProvisioned, provisioned)
			assert.Equal(t, tc.wantInstalled, installed)
		})
	}
}

func TestSourcePathRoundTrip(t *testing.T) {
	st := envState{}
	require.NoError(t, setSourcePath(st, "/abs/path/to/config.yaml"))

	got, ok := sourcePath(st)
	require.True(t, ok)
	assert.Equal(t, "/abs/path/to/config.yaml", got)
}

func TestSourcePathMissing(t *testing.T) {
	_, ok := sourcePath(envState{})
	assert.False(t, ok)
}

func TestWriteStateFileAtomicCreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "state.json")

	require.NoError(t, writeStateFileAtomic(path, envState{"kubernetesCluster": json.RawMessage(`{}`)}))

	got, err := readStateFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(got["kubernetesCluster"]))
}
