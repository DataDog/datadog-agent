// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
)

var testRawConfig = `
hostname retail
enable password cisco123
username someuser password 0 cg6#107X
aaa some-model
`

func newTestConfigStore(t *testing.T) *configStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	cs, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { cs.Close(context.Background()) })
	return cs.(*configStore)
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
	t.Run("stores and returns a UUID, hash, and stored flag", func(t *testing.T) {
		cs := newTestConfigStore(t)
		configUUID, rawHash, stored, err := cs.StoreConfig("device:10.0.0.1", "running", testRawConfig)
		require.NoError(t, err)
		assert.NotEmpty(t, configUUID)
		assert.Equal(t, HashConfig(testRawConfig), rawHash)
		assert.True(t, stored)
	})

	t.Run("each call for a device generates a unique UUID", func(t *testing.T) {
		cs := newTestConfigStore(t)
		uuid1, _, _, err := cs.StoreConfig("device:10.0.0.1", "running", testRawConfig)
		require.NoError(t, err)
		uuid2, _, _, err := cs.StoreConfig("device:10.0.0.2", "running", testRawConfig)
		require.NoError(t, err)
		assert.NotEqual(t, uuid1, uuid2)
	})

	t.Run("device deduplicate returns UUID and hash of latest config if matches", func(t *testing.T) {
		cs := newTestConfigStore(t)
		uuid1, hash1, stored1, err := cs.StoreConfig("device:10.0.0.1", "running", testRawConfig)
		require.NoError(t, err)
		assert.True(t, stored1)
		uuid2, hash2, stored2, err := cs.StoreConfig("device:10.0.0.1", "running", testRawConfig) // the same exact one, should return the first UUID (uuid1)
		require.NoError(t, err)
		assert.False(t, stored2, "duplicate write should report stored=false")
		assert.Equal(t, uuid1, uuid2)
		assert.Equal(t, hash1, hash2)
	})
}

func TestGetConfig(t *testing.T) {
	t.Run("retrieves stored config", func(t *testing.T) {
		cs := newTestConfigStore(t)
		configUUID, _, _, err := cs.StoreConfig("device:10.0.0.1", "running", testRawConfig)
		require.NoError(t, err)

		rawConfig, metadata, err := cs.GetConfig(configUUID)
		require.NoError(t, err)

		assert.Equal(t, testRawConfig, rawConfig)

		assert.Equal(t, configUUID, metadata.ConfigUUID)
		assert.Equal(t, "device:10.0.0.1", metadata.DeviceID)
		assert.Equal(t, types.RUNNING, metadata.ConfigType)
		assert.NotZero(t, metadata.CapturedAt)
		assert.Equal(t, metadata.CapturedAt, metadata.LastAccessedAt)
		assert.Equal(t, HashConfig(testRawConfig), metadata.RawHash)
		assert.NotEmpty(t, metadata.AgentVersion)
	})

	t.Run("returns error for nonexistent UUID", func(t *testing.T) {
		cs := newTestConfigStore(t)
		_, _, err := cs.GetConfig("nonexistent-uuid")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("gets configs by UUID (two different configs)", func(t *testing.T) {
		cs := newTestConfigStore(t)
		uuid1, _, _, err := cs.StoreConfig("device:10.0.0.1", "running", "config-one")
		require.NoError(t, err)
		uuid2, _, _, err := cs.StoreConfig("device:10.0.0.2", "startup", "config-two")
		require.NoError(t, err)

		raw1, meta1, err := cs.GetConfig(uuid1)
		require.NoError(t, err)
		assert.Equal(t, "config-one", raw1)
		assert.Equal(t, "device:10.0.0.1", meta1.DeviceID)

		raw2, meta2, err := cs.GetConfig(uuid2)
		require.NoError(t, err)
		assert.Equal(t, "config-two", raw2)
		assert.Equal(t, "device:10.0.0.2", meta2.DeviceID)
	})
}

func TestDeleteConfig(t *testing.T) {
	t.Run("deletes config from all buckets", func(t *testing.T) {
		cs := newTestConfigStore(t)
		configUUID, _, _, err := cs.StoreConfig("device:10.0.0.1", "running", testRawConfig)
		require.NoError(t, err)

		err = cs.DeleteConfig(configUUID)
		require.NoError(t, err)

		_, _, err = cs.GetConfig(configUUID)
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
		uuid1, _, _, err := cs.StoreConfig("device:10.0.0.1", "running", testRawConfig)
		require.NoError(t, err)
		uuid2, _, _, err := cs.StoreConfig("device:10.0.0.2", "running", testRawConfig)
		require.NoError(t, err)

		err = cs.DeleteConfig(uuid1)
		require.NoError(t, err)

		_, _, err = cs.GetConfig(uuid2)
		require.NoError(t, err)
	})
}

func TestValidateStoreConfigValues(t *testing.T) {
	tests := []struct {
		name          string
		min           int
		max           int
		maxBytes      int64
		expectedMin   int
		expectedMax   int
		expectedBytes int64
	}{
		{
			name:          "valid values pass through unchanged",
			min:           3,
			max:           15,
			maxBytes:      1000000,
			expectedMin:   3,
			expectedMax:   15,
			expectedBytes: 1000000,
		},
		{
			name:          "zero values fall back to defaults",
			min:           0,
			max:           0,
			maxBytes:      0,
			expectedMin:   defaultMinConfigsPerDevice,
			expectedMax:   defaultMaxConfigsPerDevice,
			expectedBytes: defaultMaxRawConfigStoreBytes,
		},
		{
			name:          "negative values fall back to defaults",
			min:           -1,
			max:           -5,
			maxBytes:      -100,
			expectedMin:   defaultMinConfigsPerDevice,
			expectedMax:   defaultMaxConfigsPerDevice,
			expectedBytes: defaultMaxRawConfigStoreBytes,
		},
		{
			name:          "min greater than max resets both to defaults",
			min:           20,
			max:           10,
			maxBytes:      5000,
			expectedMin:   defaultMinConfigsPerDevice,
			expectedMax:   defaultMaxConfigsPerDevice,
			expectedBytes: 5000,
		},
		{
			name:          "min equal to max is valid",
			min:           7,
			max:           7,
			maxBytes:      999,
			expectedMin:   7,
			expectedMax:   7,
			expectedBytes: 999,
		},
		{
			name:          "only min invalid falls back, max kept",
			min:           0,
			max:           20,
			maxBytes:      5000,
			expectedMin:   defaultMinConfigsPerDevice,
			expectedMax:   20,
			expectedBytes: 5000,
		},
		{
			name:          "only max invalid falls back, min kept",
			min:           3,
			max:           0,
			maxBytes:      5000,
			expectedMin:   3,
			expectedMax:   defaultMaxConfigsPerDevice,
			expectedBytes: 5000,
		},
		{
			name:          "only maxBytes invalid falls back",
			min:           3,
			max:           15,
			maxBytes:      -1,
			expectedMin:   3,
			expectedMax:   15,
			expectedBytes: defaultMaxRawConfigStoreBytes,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validatedMin, validatedMax, validatedBytes := validateStoreConfigValues(tt.min, tt.max, tt.maxBytes)
			assert.Equal(t, tt.expectedMin, validatedMin, "min")
			assert.Equal(t, tt.expectedMax, validatedMax, "max")
			assert.Equal(t, tt.expectedBytes, validatedBytes, "maxBytes")
		})
	}
}

func TestUpdateStoreConfig(t *testing.T) {
	tests := []struct {
		name          string
		min           int
		max           int
		maxBytes      int64
		expectedMin   int
		expectedMax   int
		expectedBytes int64
	}{
		{
			name:          "applies valid custom values",
			min:           2,
			max:           20,
			maxBytes:      9999,
			expectedMin:   2,
			expectedMax:   20,
			expectedBytes: 9999,
		},
		{
			name:          "keeps defaults when zeros are passed",
			min:           0,
			max:           0,
			maxBytes:      0,
			expectedMin:   defaultMinConfigsPerDevice,
			expectedMax:   defaultMaxConfigsPerDevice,
			expectedBytes: defaultMaxRawConfigStoreBytes,
		},
		{
			name:          "resets min and max when min exceeds max",
			min:           50,
			max:           10,
			maxBytes:      5000,
			expectedMin:   defaultMinConfigsPerDevice,
			expectedMax:   defaultMaxConfigsPerDevice,
			expectedBytes: 5000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := newTestConfigStore(t)
			cs.UpdateStoreConfig(tt.min, tt.max, tt.maxBytes)

			assert.Equal(t, tt.expectedMin, cs.minConfigsPerDevice, "min")
			assert.Equal(t, tt.expectedMax, cs.maxConfigsPerDevice, "max")
			assert.Equal(t, tt.expectedBytes, cs.maxRawConfigStoreBytes, "maxBytes")
		})
	}

	t.Run("successive updates apply correctly", func(t *testing.T) {
		cs := newTestConfigStore(t)
		cs.UpdateStoreConfig(3, 30, 1000)
		assert.Equal(t, 3, cs.minConfigsPerDevice)

		cs.UpdateStoreConfig(6, 60, 2000)
		assert.Equal(t, 6, cs.minConfigsPerDevice)
		assert.Equal(t, 60, cs.maxConfigsPerDevice)
		assert.Equal(t, int64(2000), cs.maxRawConfigStoreBytes)
	})
}

func TestOpenSetsDefaults(t *testing.T) {
	cs := newTestConfigStore(t)
	assert.Equal(t, defaultMinConfigsPerDevice, cs.minConfigsPerDevice)
	assert.Equal(t, defaultMaxConfigsPerDevice, cs.maxConfigsPerDevice)
	assert.Equal(t, defaultMaxRawConfigStoreBytes, cs.maxRawConfigStoreBytes)
}

func TestHashConfig(t *testing.T) {
	t.Run("deterministic", func(t *testing.T) {
		h1 := HashConfig("hello")
		h2 := HashConfig("hello")
		assert.Equal(t, h1, h2)
	})

	t.Run("different inputs produce different hashes", func(t *testing.T) {
		h1 := HashConfig("config-a")
		h2 := HashConfig("config-b")
		assert.NotEqual(t, h1, h2)
	})
}

func TestCheckDuplicate(t *testing.T) {
	tests := []struct {
		name           string
		existing       []types.ConfigMetadata // seed these into the metadata bucket
		deviceID       string
		configType     types.ConfigType
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
			existing: []types.ConfigMetadata{
				{ConfigUUID: "uuid-1", DeviceID: "device:10.0.0.1", ConfigType: "running", CapturedAt: 100, RawHash: "abc123"},
			},
			deviceID:       "device:10.0.0.1",
			configType:     "running",
			rawHash:        "abc123",
			wantConfigUUID: "uuid-1",
		},
		{
			name: "same device, but different hash returns no match (new config, not duplicate)",
			existing: []types.ConfigMetadata{
				{ConfigUUID: "uuid-1", DeviceID: "device:10.0.0.1", ConfigType: "running", CapturedAt: 100, RawHash: "abc123"},
			},
			deviceID:       "device:10.0.0.1",
			configType:     "running",
			rawHash:        "different-hash",
			wantConfigUUID: "",
		},
		{
			name: "matches latest by CapturedAt (duplicate of the latest config)",
			existing: []types.ConfigMetadata{
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
			existing: []types.ConfigMetadata{
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
			existing: []types.ConfigMetadata{
				{ConfigUUID: "uuid-1", DeviceID: "device:10.0.0.2", ConfigType: "running", CapturedAt: 100, RawHash: "abc123"},
			},
			deviceID:       "device:10.0.0.1",
			configType:     "running",
			rawHash:        "abc123",
			wantConfigUUID: "",
		},
		{
			name: "different config type not matched (same config, same device, but different config type should store as new config)",
			existing: []types.ConfigMetadata{
				{ConfigUUID: "uuid-1", DeviceID: "device:10.0.0.1", ConfigType: "startup", CapturedAt: 100, RawHash: "abc123"},
			},
			deviceID:       "device:10.0.0.1",
			configType:     "running",
			rawHash:        "abc123",
			wantConfigUUID: "",
		},
		{
			name: "existing metadata has same capturedAt ts - should break by config UUID order",
			existing: []types.ConfigMetadata{
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

func TestGetAllConfigMetadata(t *testing.T) {
	t.Run("empty store returns no entries", func(t *testing.T) {
		cs := newTestConfigStore(t)
		configMeta, err := cs.GetAllConfigMetadata()
		require.NoError(t, err)
		assert.Empty(t, configMeta)
	})

	t.Run("returns entries for multiple devices and types", func(t *testing.T) {
		cs := newTestConfigStore(t)
		uuid1, _, _, err := cs.StoreConfig("device:10.0.0.1", types.RUNNING, "running-1")
		require.NoError(t, err)
		uuid2, _, _, err := cs.StoreConfig("device:10.0.0.1", types.STARTUP, "startup-1")
		require.NoError(t, err)
		uuid3, _, _, err := cs.StoreConfig("device:10.0.0.2", types.RUNNING, "running-2")
		require.NoError(t, err)

		configMeta, err := cs.GetAllConfigMetadata()
		require.NoError(t, err)
		require.Len(t, configMeta, 3)

		configMetaUUIDs := []string{configMeta[0].ConfigUUID, configMeta[1].ConfigUUID, configMeta[2].ConfigUUID}
		assert.ElementsMatch(t, []string{uuid1, uuid2, uuid3}, configMetaUUIDs)
	})
	t.Run("populates all metadata fields", func(t *testing.T) {
		cs := newTestConfigStore(t)
		uuid, _, _, err := cs.StoreConfig("device:10.0.0.1", types.RUNNING, testRawConfig)
		require.NoError(t, err)

		configMeta, err := cs.GetAllConfigMetadata()
		require.NoError(t, err)
		require.Len(t, configMeta, 1)
		assert.Equal(t, uuid, configMeta[0].ConfigUUID)
		assert.Equal(t, "device:10.0.0.1", configMeta[0].DeviceID)
		assert.Equal(t, types.RUNNING, configMeta[0].ConfigType)
		assert.NotZero(t, configMeta[0].CapturedAt)
		assert.Equal(t, HashConfig(testRawConfig), configMeta[0].RawHash)
		assert.NotEmpty(t, configMeta[0].AgentVersion)
	})

	t.Run("reflects deletes", func(t *testing.T) {
		cs := newTestConfigStore(t)
		uuid1, _, _, err := cs.StoreConfig("device:10.0.0.1", types.RUNNING, "config-a")
		require.NoError(t, err)
		uuid2, _, _, err := cs.StoreConfig("device:10.0.0.2", types.RUNNING, "config-b")
		require.NoError(t, err)

		require.NoError(t, cs.DeleteConfig(uuid1))

		configMeta, err := cs.GetAllConfigMetadata()
		require.NoError(t, err)
		require.Len(t, configMeta, 1)
		configMeta2, err := cs.GetAllConfigMetadata()
		require.NoError(t, err)
		assert.Equal(t, uuid2, configMeta2[0].ConfigUUID)
	})
}
