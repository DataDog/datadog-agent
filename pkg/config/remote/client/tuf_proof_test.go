// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"testing"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigTUFProofTracksRootsTargetsAndFiles(t *testing.T) {
	const targetPath = "datadog/42/AP_RUNNER_KEYS/key-1/config"
	client := &Client{proofTargetFiles: make(map[string][]byte)}
	client.storeConfigTUFProof(&pbgo.ClientGetConfigsResponse{
		Roots:         [][]byte{[]byte("root-2")},
		Targets:       []byte("targets-1"),
		TargetFiles:   []*pbgo.File{{Path: targetPath, Raw: []byte("key-1")}},
		ClientConfigs: []string{targetPath},
	})
	client.storeConfigTUFProof(&pbgo.ClientGetConfigsResponse{
		Roots:         [][]byte{[]byte("root-3")},
		Targets:       []byte("targets-2"),
		ClientConfigs: []string{targetPath},
	})

	proof, ok := client.GetConfigTUFProof(targetPath)
	require.True(t, ok)
	assert.Equal(t, [][]byte{[]byte("root-2"), []byte("root-3")}, proof.Roots)
	assert.Equal(t, []byte("targets-2"), proof.Targets)
	assert.Equal(t, targetPath, proof.TargetPath)
	assert.Equal(t, []byte("key-1"), proof.TargetFile)

	proof.Roots[0][0] = 'x'
	proof.Targets[0] = 'x'
	proof.TargetFile[0] = 'x'
	stored, ok := client.GetConfigTUFProof(targetPath)
	require.True(t, ok)
	assert.Equal(t, []byte("root-2"), stored.Roots[0])
	assert.Equal(t, []byte("targets-2"), stored.Targets)
	assert.Equal(t, []byte("key-1"), stored.TargetFile)
}

func TestConfigTUFProofRemovesAbsentTarget(t *testing.T) {
	const targetPath = "datadog/42/AP_RUNNER_KEYS/key-1/config"
	client := &Client{proofTargetFiles: make(map[string][]byte)}
	client.storeConfigTUFProof(&pbgo.ClientGetConfigsResponse{
		Targets:       []byte("targets-1"),
		TargetFiles:   []*pbgo.File{{Path: targetPath, Raw: []byte("key-1")}},
		ClientConfigs: []string{targetPath},
	})
	client.storeConfigTUFProof(&pbgo.ClientGetConfigsResponse{Targets: []byte("targets-2")})

	_, ok := client.GetConfigTUFProof(targetPath)
	assert.False(t, ok)
}
