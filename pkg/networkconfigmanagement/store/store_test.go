// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && ncm

package store

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
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
		cs := newTestConfigStore(t)
		uuid1, err := cs.StoreConfig("device:10.0.0.1", "running", testRawConfig, testBlocks, testSecrets)
		require.NoError(t, err)
		uuid2, err := cs.StoreConfig("device:10.0.0.2", "running", testRawConfig, testBlocks, testSecrets)
		require.NoError(t, err)
		assert.NotEqual(t, uuid1, uuid2)
	})

	t.Run("device deduplicate returns UUID of latest config if matches", func(t *testing.T) {
		cs := newTestConfigStore(t)
		uuid1, err := cs.StoreConfig("device:10.0.0.1", "running", testRawConfig, testBlocks, testSecrets)
		require.NoError(t, err)
		uuid2, err := cs.StoreConfig("device:10.0.0.1", "running", testRawConfig, testBlocks, testSecrets) // the same exact one, should return the first UUID (uuid1)
		require.NoError(t, err)
		assert.Equal(t, uuid1, uuid2)
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

func TestCheckDuplicate(t *testing.T) {
	tests := []struct {
		name           string
		existing       []ConfigMetadata // seed these into the metadata bucket
		deviceID       string
		configType     string
		rawHash        string
		wantConfigUUID string // empty means no match expected
	}{
		{
			name:           "empty store returns no match",
			deviceID:       "device:10.0.0.1",
			configType:     "running",
			rawHash:        "abc123",
			wantConfigUUID: "",
		},
		{
			name: "matching hash returns UUID (duplicate config)",
			existing: []ConfigMetadata{
				{ConfigUUID: "uuid-1", DeviceID: "device:10.0.0.1", ConfigType: "running", CapturedAt: 100, RawHash: "abc123"},
			},
			deviceID:       "device:10.0.0.1",
			configType:     "running",
			rawHash:        "abc123",
			wantConfigUUID: "uuid-1",
		},
		{
			name: "same device, but different hash returns no match (new config, not duplicate)",
			existing: []ConfigMetadata{
				{ConfigUUID: "uuid-1", DeviceID: "device:10.0.0.1", ConfigType: "running", CapturedAt: 100, RawHash: "abc123"},
			},
			deviceID:       "device:10.0.0.1",
			configType:     "running",
			rawHash:        "different-hash",
			wantConfigUUID: "",
		},
		{
			name: "matches latest by CapturedAt (duplicate of the latest config)",
			existing: []ConfigMetadata{
				{ConfigUUID: "uuid-old", DeviceID: "device:10.0.0.1", ConfigType: "running", CapturedAt: 100, RawHash: "old-hash"},
				{ConfigUUID: "uuid-new", DeviceID: "device:10.0.0.1", ConfigType: "running", CapturedAt: 200, RawHash: "new-hash"},
			},
			deviceID:       "device:10.0.0.1",
			configType:     "running",
			rawHash:        "new-hash",
			wantConfigUUID: "uuid-new",
		},
		{
			name: "old hash no longer matches when latest differs (seen hash, but not duplicate since it doesn't match latest)",
			existing: []ConfigMetadata{
				{ConfigUUID: "uuid-old", DeviceID: "device:10.0.0.1", ConfigType: "running", CapturedAt: 100, RawHash: "old-hash"},
				{ConfigUUID: "uuid-new", DeviceID: "device:10.0.0.1", ConfigType: "running", CapturedAt: 200, RawHash: "new-hash"},
			},
			deviceID:       "device:10.0.0.1",
			configType:     "running",
			rawHash:        "old-hash",
			wantConfigUUID: "",
		},
		{
			name: "different device not matched (same hash, but different device)",
			existing: []ConfigMetadata{
				{ConfigUUID: "uuid-1", DeviceID: "device:10.0.0.2", ConfigType: "running", CapturedAt: 100, RawHash: "abc123"},
			},
			deviceID:       "device:10.0.0.1",
			configType:     "running",
			rawHash:        "abc123",
			wantConfigUUID: "",
		},
		{
			name: "different config type not matched (same config, same device, but different config type should store as new config)",
			existing: []ConfigMetadata{
				{ConfigUUID: "uuid-1", DeviceID: "device:10.0.0.1", ConfigType: "startup", CapturedAt: 100, RawHash: "abc123"},
			},
			deviceID:       "device:10.0.0.1",
			configType:     "running",
			rawHash:        "abc123",
			wantConfigUUID: "",
		},
		{
			name: "existing metadata has same capturedAt ts - should break by config UUID order",
			existing: []ConfigMetadata{
				{ConfigUUID: "uuid-1", DeviceID: "device:10.0.0.1", ConfigType: "running", CapturedAt: 100, RawHash: "hash123"},
				{ConfigUUID: "uuid-2", DeviceID: "device:10.0.0.1", ConfigType: "running", CapturedAt: 100, RawHash: "hash345"}, // will win from config UUID
			},
			deviceID:       "device:10.0.0.1",
			configType:     "running",
			rawHash:        "hash678",
			wantConfigUUID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := newTestConfigStore(t)

			// Add the "existing" metadata
			// TODO: if 2+ times, maybe extract each bucket put into separate fn for ease of testing, etc.
			if len(tt.existing) > 0 {
				err := cs.db.Update(func(tx *bbolt.Tx) error {
					bucket := tx.Bucket([]byte(metadataBucket))
					for _, meta := range tt.existing {
						val, err := json.Marshal(meta)
						if err != nil {
							return err
						}
						if err := bucket.Put([]byte(meta.ConfigUUID), val); err != nil {
							return err
						}
					}
					return nil
				})
				require.NoError(t, err)
			}

			gotUUID, err := cs.CheckDuplicate(tt.deviceID, tt.configType, tt.rawHash)
			require.NoError(t, err)
			assert.Equal(t, tt.wantConfigUUID, gotUUID)
		})
	}
}
