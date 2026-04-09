// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && ncm

package store

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testRawConfig = `
hostname retail
enable password cisco123
username someuser password 0 cg6#107X
aaa some-model
`
var testBlocks = []ConfigBlock{
	{Type: TextBlock, Value: "hostname retail\nenable password "},
	{Type: SecretBlock, ID: "secret-1"},
	{Type: TextBlock, Value: "\nusername someuser password 0 "},
	{Type: SecretBlock, ID: "secret-2"},
	{Type: TextBlock, Value: "\naaa some-model"},
}

var testSecrets = map[string]string{
	"secret-1": "cisco123",
	"secret-2": "cg6#107X",
}

func newTestConfigStore(t *testing.T) *ConfigStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	cs, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { cs.Close() })
	return cs
}

func TestOpen(t *testing.T) {
	t.Run("creates db and buckets successfully", func(t *testing.T) {
		cs := newTestConfigStore(t)
		assert.NotNil(t, cs)
		assert.NotNil(t, cs.db)
	})

	t.Run("fails on invalid path", func(t *testing.T) {
		_, err := Open("/nonexistent/dir/test.db")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open NCM bbolt config store")
	})
}

func TestStoreConfig(t *testing.T) {
	t.Run("stores and returns a UUID", func(t *testing.T) {
		cs := newTestConfigStore(t)
		configUUID, err := cs.StoreConfig("device:10.0.0.1", "running", testRawConfig, testBlocks, testSecrets)
		require.NoError(t, err)
		assert.NotEmpty(t, configUUID)
	})

	t.Run("each call for a device generates a unique UUID", func(t *testing.T) {
		// TODO: update to add test w/ de-dupe (should return same UUID as the last config it matches)
		cs := newTestConfigStore(t)
		uuid1, err := cs.StoreConfig("device:10.0.0.1", "running", testRawConfig, testBlocks, testSecrets)
		require.NoError(t, err)
		uuid2, err := cs.StoreConfig("device:10.0.0.2", "running", testRawConfig, testBlocks, testSecrets)
		require.NoError(t, err)
		assert.NotEqual(t, uuid1, uuid2)
	})
}

func TestGetConfig(t *testing.T) {
	t.Run("retrieves stored config", func(t *testing.T) {
		cs := newTestConfigStore(t)
		configUUID, err := cs.StoreConfig("device:10.0.0.1", "running", testRawConfig, testBlocks, testSecrets)
		require.NoError(t, err)

		rawConfig, blocks, metadata, secrets, err := cs.GetConfig(configUUID)
		require.NoError(t, err)

		assert.Equal(t, testRawConfig, rawConfig)
		assert.Equal(t, testBlocks, blocks)
		assert.Equal(t, testSecrets, secrets)

		assert.Equal(t, configUUID, metadata.ConfigUUID)
		assert.Equal(t, "device:10.0.0.1", metadata.DeviceID)
		assert.Equal(t, "running", metadata.ConfigType)
		assert.NotZero(t, metadata.CapturedAt)
		assert.Equal(t, metadata.CapturedAt, metadata.LastAccessedAt)
		assert.Equal(t, hashConfig(testRawConfig), metadata.RawHash)
		assert.NotEmpty(t, metadata.AgentVersion)
	})

	t.Run("returns error for nonexistent UUID", func(t *testing.T) {
		cs := newTestConfigStore(t)
		_, _, _, _, err := cs.GetConfig("nonexistent-uuid")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("gets configs by UUID (two different configs)", func(t *testing.T) {
		cs := newTestConfigStore(t)
		uuid1, err := cs.StoreConfig("device:10.0.0.1", "running", "config-one", nil, map[string]string{"a": "1"})
		require.NoError(t, err)
		uuid2, err := cs.StoreConfig("device:10.0.0.2", "startup", "config-two", nil, map[string]string{"b": "2"})
		require.NoError(t, err)

		raw1, _, meta1, secrets1, err := cs.GetConfig(uuid1)
		require.NoError(t, err)
		assert.Equal(t, "config-one", raw1)
		assert.Equal(t, "device:10.0.0.1", meta1.DeviceID)
		assert.Equal(t, map[string]string{"a": "1"}, secrets1)

		raw2, _, meta2, secrets2, err := cs.GetConfig(uuid2)
		require.NoError(t, err)
		assert.Equal(t, "config-two", raw2)
		assert.Equal(t, "device:10.0.0.2", meta2.DeviceID)
		assert.Equal(t, map[string]string{"b": "2"}, secrets2)
	})
}

func TestDeleteConfig(t *testing.T) {
	t.Run("deletes config from all buckets", func(t *testing.T) {
		cs := newTestConfigStore(t)
		configUUID, err := cs.StoreConfig("device:10.0.0.1", "running", testRawConfig, testBlocks, testSecrets)
		require.NoError(t, err)

		err = cs.DeleteConfig(configUUID)
		require.NoError(t, err)

		_, _, _, _, err = cs.GetConfig(configUUID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("returns error for nonexistent key", func(t *testing.T) {
		cs := newTestConfigStore(t)
		err := cs.DeleteConfig("nonexistent-uuid")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("deleting one config does not affect another", func(t *testing.T) {
		cs := newTestConfigStore(t)
		uuid1, err := cs.StoreConfig("device:10.0.0.1", "running", testRawConfig, testBlocks, testSecrets)
		require.NoError(t, err)
		uuid2, err := cs.StoreConfig("device:10.0.0.2", "running", testRawConfig, testBlocks, testSecrets)
		require.NoError(t, err)

		err = cs.DeleteConfig(uuid1)
		require.NoError(t, err)

		_, _, _, _, err = cs.GetConfig(uuid2)
		require.NoError(t, err)
	})
}

func TestHashConfig(t *testing.T) {
	t.Run("deterministic", func(t *testing.T) {
		h1 := hashConfig("hello")
		h2 := hashConfig("hello")
		assert.Equal(t, h1, h2)
	})

	t.Run("different inputs produce different hashes", func(t *testing.T) {
		h1 := hashConfig("config-a")
		h2 := hashConfig("config-b")
		assert.NotEqual(t, h1, h2)
	})
}
