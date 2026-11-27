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

	rcmock "github.com/DataDog/datadog-agent/pkg/config/remote/client/mock"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	wantProof := state.ConfigTUFProof{
		Roots:      [][]byte{[]byte("root-2")},
		Targets:    []byte("targets"),
		TargetPath: targetPath,
		TargetFile: rawKey,
	}
	rcClient := rcmock.New()
	rcClient.SetConfigTUFProof(wantProof)
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
	assert.Equal(t, wantProof.Roots, proof.Roots)
	assert.Equal(t, wantProof.Targets, proof.Targets)
	assert.Equal(t, wantProof.TargetPath, proof.TargetPath)
	assert.Equal(t, wantProof.TargetFile, proof.TargetFile)
}

func TestEagerKeysManagerReturnsDirectorProofAfterProviderAttached(t *testing.T) {
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

	wantProof := state.ConfigTUFProof{
		Roots:      [][]byte{[]byte("root-2")},
		Targets:    []byte("targets"),
		TargetPath: targetPath,
		TargetFile: rawKey,
	}
	rcClient := rcmock.New()
	rcClient.SetConfigTUFProof(wantProof)
	manager, callback := NewKeyManagerWithCallback()
	callback(map[string]state.RawConfig{
		targetPath: {
			Config: rawKey,
			Metadata: state.Metadata{
				ID:      "key-1",
				Product: state.ProductActionPlatformRunnerKeys,
			},
		},
	}, func(string, state.ApplyStatus) {})

	SetProofProvider(manager, rcClient)

	key, proof := manager.GetKey("key-1")
	require.NotNil(t, key)
	require.NotNil(t, proof)
	assert.Equal(t, wantProof.TargetPath, proof.TargetPath)
}
