// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package store

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/benbjohnson/clock"
	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type memConfigStore struct {
	lock       sync.RWMutex
	rawConfigs map[string]string
	metadata   map[string]types.ConfigMetadata
	clock      clock.Clock
	uuidGen    func() string

	minConfigsPerDevice    int
	maxConfigsPerDevice    int
	maxRawConfigStoreBytes int64
}

var _ ConfigStore = (*memConfigStore)(nil)

// MemStoreOption configures the memstore at construction.
type MemStoreOption func(*memConfigStore)

// WithClock overrides the clock used to stamp CapturedAt / LastAccessedAt.
// Useful in tests that need deterministic timestamps.
func WithClock(c clock.Clock) MemStoreOption {
	return func(m *memConfigStore) { m.clock = c }
}

// WithUUIDGenerator overrides the UUID source for new entries. Useful in tests
// that need deterministic ConfigUUIDs.
func WithUUIDGenerator(gen func() string) MemStoreOption {
	return func(m *memConfigStore) { m.uuidGen = gen }
}

// NewMemStore creates a ConfigStore backed by in-memory maps (for use in tests).
func NewMemStore(opts ...MemStoreOption) ConfigStore {
	m := &memConfigStore{
		rawConfigs:             make(map[string]string),
		metadata:               make(map[string]types.ConfigMetadata),
		clock:                  clock.New(),
		uuidGen:                func() string { return uuid.New().String() },
		minConfigsPerDevice:    defaultMinConfigsPerDevice,
		maxConfigsPerDevice:    defaultMaxConfigsPerDevice,
		maxRawConfigStoreBytes: defaultMaxRawConfigStoreBytes,
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

// Close is a no-op for the in-memory store.
func (m *memConfigStore) Close(_ context.Context) error {
	return nil
}

// StoreConfig stores a device configuration, deduplicating against the latest stored config for the same device+type.
// Returns the config UUID, the SHA-256 hash of the raw config, and whether a new entry was written (false for duplicates).
func (m *memConfigStore) StoreConfig(deviceID string, configType types.ConfigType, rawConfig string) (string, string, bool, error) {
	rawHash := HashConfig(rawConfig)
	now := m.clock.Now().Unix()

	m.lock.Lock()
	defer m.lock.Unlock()

	if existingID := m.findLatestMatch(deviceID, configType, rawHash); existingID != "" {
		return existingID, rawHash, false, nil
	}

	configUUID := m.uuidGen()
	m.rawConfigs[configUUID] = rawConfig

	m.metadata[configUUID] = types.ConfigMetadata{
		ConfigUUID:     configUUID,
		DeviceID:       deviceID,
		ConfigType:     configType,
		CapturedAt:     now,
		LastAccessedAt: now,
		RawHash:        rawHash,
		AgentVersion:   version.AgentVersion,
	}

	return configUUID, rawHash, true, nil
}

// findLatestMatch returns the UUID of the latest stored config for the given device+type if its hash matches.
// Must be called with m.lock held.
func (m *memConfigStore) findLatestMatch(deviceID string, configType types.ConfigType, rawHash string) string {
	var latest *types.ConfigMetadata
	for _, meta := range m.metadata {
		if meta.DeviceID != deviceID || meta.ConfigType != configType {
			continue
		}
		if latest == nil || meta.CapturedAt > latest.CapturedAt || (meta.CapturedAt == latest.CapturedAt && meta.ConfigUUID > latest.ConfigUUID) {
			latest = &meta
		}
	}
	if latest != nil && latest.RawHash == rawHash {
		return latest.ConfigUUID
	}
	return ""
}

// CheckDuplicate returns the UUID of the latest stored config for the given device+type if its hash matches, or empty string otherwise.
func (m *memConfigStore) CheckDuplicate(deviceID string, configType types.ConfigType, rawHash string) (string, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.findLatestMatch(deviceID, configType, rawHash), nil
}

// UpdateStoreConfig validates and applies new eviction-policy knobs
func (m *memConfigStore) UpdateStoreConfig(minConfigsPerDevice, maxConfigsPerDevice int, maxRawConfigStoreBytes int64) {
	minConfigsPerDevice, maxConfigsPerDevice, maxRawConfigStoreBytes = validateStoreConfigValues(minConfigsPerDevice, maxConfigsPerDevice, maxRawConfigStoreBytes)

	m.lock.Lock()
	defer m.lock.Unlock()

	m.minConfigsPerDevice = minConfigsPerDevice
	m.maxConfigsPerDevice = maxConfigsPerDevice
	m.maxRawConfigStoreBytes = maxRawConfigStoreBytes
}

// GetConfig retrieves all data for a config by UUID.
func (m *memConfigStore) GetConfig(configUUID string) (string, *types.ConfigMetadata, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	rawConfig, ok := m.rawConfigs[configUUID]
	if !ok {
		return "", nil, fmt.Errorf("raw config not found for UUID: %s", configUUID)
	}

	meta := m.metadata[configUUID]

	return rawConfig, &meta, nil
}

// GetAllConfigMetadata returns metadata for every stored config across all devices,
// sorted by ConfigUUID for deterministic ordering.
func (m *memConfigStore) GetAllConfigMetadata() ([]*types.ConfigMetadata, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	configMeta := make([]*types.ConfigMetadata, 0, len(m.metadata))
	for _, value := range m.metadata {
		v := value
		configMeta = append(configMeta, &v)
	}
	sort.Slice(configMeta, func(i, j int) bool {
		return configMeta[i].ConfigUUID < configMeta[j].ConfigUUID
	})
	return configMeta, nil
}
