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
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

func newTestKeysManager() *keysManager {
	return &keysManager{
		keys:         make(map[string]types.DecodedKey),
		readyChanged: make(chan struct{}),
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

func TestKeysManagerAuthoritativeEmptySnapshotIsReady(t *testing.T) {
	manager := newTestKeysManager()
	require.NoError(t, manager.InstallAuthoritative(nil))
	require.NoError(t, manager.WaitForReady(context.Background()))
	require.True(t, manager.IsReady())
}

func TestKeysManagerExpirationAndRecovery(t *testing.T) {
	manager := newTestKeysManager()
	key := testSigningKey(t, "key")
	require.NoError(t, manager.InstallAuthoritative([]SigningKey{key}))
	require.NotNil(t, manager.GetKey(key.ID))

	manager.MarkExpired()
	require.False(t, manager.IsReady())
	require.NotNil(t, manager.GetKey(key.ID), "expiration preserves the last valid snapshot")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, manager.WaitForReady(ctx), context.Canceled)

	require.NoError(t, manager.InstallAuthoritative([]SigningKey{key}))
	require.True(t, manager.IsReady())
}

func TestKeysManagerRemoteConfigUpdateIsAtomic(t *testing.T) {
	manager := newTestKeysManager()
	valid := testSigningKey(t, "valid")
	require.NoError(t, manager.InstallAuthoritative([]SigningKey{valid}))

	invalid := state.RawConfig{Metadata: state.Metadata{ID: "invalid"}, Config: []byte(`{"keyType":"ED25519","key":"bad"}`)}
	manager.AgentConfigUpdateCallback(
		map[string]state.RawConfig{"invalid": invalid},
		func(_ string, status state.ApplyStatus) { require.Equal(t, state.ApplyStateError, status.State) },
	)

	require.NotNil(t, manager.GetKey(valid.ID))
	require.Nil(t, manager.GetKey("invalid"))
}

func TestKeysManagerWaitForReadyHonorsCancellation(t *testing.T) {
	manager := newTestKeysManager()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, manager.WaitForReady(ctx), context.Canceled)
}
