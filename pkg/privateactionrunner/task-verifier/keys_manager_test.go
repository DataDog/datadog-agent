// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package taskverifier

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type proofRCClient struct {
	proof state.ConfigTUFProof
}

func (c *proofRCClient) Subscribe(string, func(map[string]state.RawConfig, func(string, state.ApplyStatus))) {
}

func (c *proofRCClient) GetConfigTUFProof(targetPath string) (state.ConfigTUFProof, bool) {
	if targetPath != c.proof.TargetPath {
		return state.ConfigTUFProof{}, false
	}
	return c.proof, true
}

func TestKeysManagerReturnsDirectorProofForKeyTarget(t *testing.T) {
	const targetPath = "datadog/42/AP_RUNNER_KEYS/key-1/config"
	public, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(public)
	require.NoError(t, err)
	rawKey, err := json.Marshal(types.RawKey{
		KeyType: types.KeyTypeED25519,
		Key:     pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}),
	})
	require.NoError(t, err)

	rcClient := &proofRCClient{proof: state.ConfigTUFProof{
		Roots:      [][]byte{[]byte("root-2")},
		Targets:    []byte("targets"),
		TargetPath: targetPath,
		TargetFile: rawKey,
	}}
	manager := NewKeyManager(rcClient).(*keysManager)
	manager.AgentConfigUpdateCallback(map[string]state.RawConfig{
		targetPath: {
			Config: rawKey,
			Metadata: state.Metadata{
				ID:      "key-1",
				Product: state.ProductActionPlatformRunnerKeys,
			},
		},
	}, func(string, state.ApplyStatus) {})

	key, proof := manager.GetKey("key-1")
	require.NotNil(t, key)
	require.NotNil(t, proof)
	assert.Equal(t, types.KeyTypeED25519, key.GetKeyType())
	assert.Equal(t, rcClient.proof.Roots, proof.Roots)
	assert.Equal(t, rcClient.proof.Targets, proof.Targets)
	assert.Equal(t, rcClient.proof.TargetPath, proof.TargetPath)
	assert.Equal(t, rcClient.proof.TargetFile, proof.TargetFile)
}
