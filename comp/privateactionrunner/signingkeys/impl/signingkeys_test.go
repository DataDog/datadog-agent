// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package signingkeysimpl

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/stretchr/testify/require"
)

func validConfig(t *testing.T, id string) state.RawConfig {
	t.Helper()
	public, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(public)
	require.NoError(t, err)
	raw, err := json.Marshal(types.RawKey{
		KeyType: types.KeyTypeED25519,
		Key:     pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}),
	})
	require.NoError(t, err)
	return state.RawConfig{Config: raw, Metadata: state.Metadata{ID: id}}
}

func acknowledge(_ string, _ state.ApplyStatus) {}

func TestAuthoritativeEmptySnapshotInitializes(t *testing.T) {
	c := &component{}
	c.onStatus(map[string]state.RawConfig{}, pbgo.ConfigStatus_CONFIG_STATUS_OK, acknowledge)

	snapshot, err := c.Get(0)
	require.NoError(t, err)
	require.True(t, snapshot.Initialized)
	require.Empty(t, snapshot.Keys)
	require.Equal(t, uint64(1), snapshot.Revision)
	require.False(t, snapshot.Unchanged)

	snapshot, err = c.Get(1)
	require.NoError(t, err)
	require.True(t, snapshot.Unchanged)
}

func TestMalformedSnapshotDoesNotReplaceValidKeys(t *testing.T) {
	c := &component{}
	valid := map[string]state.RawConfig{"valid": validConfig(t, "valid")}
	c.onStatus(valid, pbgo.ConfigStatus_CONFIG_STATUS_OK, acknowledge)
	before, err := c.Get(0)
	require.NoError(t, err)

	c.onStatus(map[string]state.RawConfig{
		"invalid": {Config: []byte(`{"keyType":"ED25519","key":"bad"}`), Metadata: state.Metadata{ID: "invalid"}},
	}, pbgo.ConfigStatus_CONFIG_STATUS_OK, acknowledge)
	_, err = c.Get(before.Revision)
	require.ErrorIs(t, err, errUnavailable)

	c.onStatus(valid, pbgo.ConfigStatus_CONFIG_STATUS_OK, acknowledge)
	after, err := c.Get(before.Revision)
	require.NoError(t, err)
	require.True(t, after.Unchanged)
	require.Equal(t, before.Keys, after.Keys)
}

func TestExpirationChangesStatusWithoutDroppingKeys(t *testing.T) {
	c := &component{}
	c.onStatus(map[string]state.RawConfig{"valid": validConfig(t, "valid")}, pbgo.ConfigStatus_CONFIG_STATUS_OK, acknowledge)
	before, err := c.Get(0)
	require.NoError(t, err)

	c.onStatus(nil, pbgo.ConfigStatus_CONFIG_STATUS_EXPIRED, acknowledge)
	expired, err := c.Get(before.Revision)
	require.NoError(t, err)
	require.Equal(t, pbgo.ConfigStatus_CONFIG_STATUS_EXPIRED, expired.ConfigStatus)
	require.Equal(t, before.Keys, expired.Keys)
	require.Equal(t, before.Revision+1, expired.Revision)
}
