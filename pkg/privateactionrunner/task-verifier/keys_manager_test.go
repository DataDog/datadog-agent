// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package taskverifier

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

func newTestKeysManager() *keysManager {
	return &keysManager{
		keys:    make(map[string]types.DecodedKey),
		rawKeys: make(map[string]SigningKey),
		ready:   make(chan struct{}),
	}
}

func testSigningKey(t *testing.T, id string) SigningKey {
	t.Helper()
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(publicKey)
	require.NoError(t, err)
	return SigningKey{
		ID:      id,
		KeyType: types.KeyTypeED25519,
		Key: pem.EncodeToMemory(&pem.Block{
			Type:  "PUBLIC KEY",
			Bytes: der,
		}),
	}
}

func rawConfig(t *testing.T, key SigningKey) state.RawConfig {
	t.Helper()
	config, err := json.Marshal(types.RawKey{KeyType: key.KeyType, Key: key.Key})
	require.NoError(t, err)
	return state.RawConfig{
		Metadata: state.Metadata{ID: key.ID},
		Config:   config,
	}
}

func TestKeysManagerSeedMakesColdExecutorReady(t *testing.T) {
	manager := newTestKeysManager()
	seed := testSigningKey(t, "seed")

	require.NoError(t, manager.Seed([]SigningKey{seed}))
	require.NoError(t, manager.WaitForReady(context.Background()))
	require.NotNil(t, manager.GetKey(seed.ID))

	snapshot := manager.Snapshot()
	require.Equal(t, []SigningKey{seed}, snapshot)
	snapshot[0].Key[0] ^= 0xff
	require.Equal(t, seed, manager.Snapshot()[0], "snapshot must not alias the manager")
}

func TestKeysManagerRemoteConfigReplacesSeed(t *testing.T) {
	manager := newTestKeysManager()
	seed := testSigningKey(t, "seed")
	fresh := testSigningKey(t, "fresh")
	require.NoError(t, manager.Seed([]SigningKey{seed}))

	manager.AgentConfigUpdateCallback(
		map[string]state.RawConfig{"fresh-config": rawConfig(t, fresh)},
		func(_ string, status state.ApplyStatus) {
			require.Equal(t, state.ApplyStateAcknowledged, status.State)
		},
	)

	require.Nil(t, manager.GetKey(seed.ID))
	require.NotNil(t, manager.GetKey(fresh.ID))
	require.Equal(t, []SigningKey{fresh}, manager.Snapshot())

	// A stale cache sent on a later SyncKeys call must never overwrite RC.
	require.NoError(t, manager.Seed([]SigningKey{seed}))
	require.Equal(t, []SigningKey{fresh}, manager.Snapshot())
}

func TestKeysManagerWaitForReadyHonorsCancellation(t *testing.T) {
	manager := newTestKeysManager()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, manager.WaitForReady(ctx), context.Canceled)
}
