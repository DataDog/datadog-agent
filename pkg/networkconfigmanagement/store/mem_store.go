// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build ncm

package store

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/pkg/version"
)

type memConfigStore struct {
	lock       sync.RWMutex
	rawConfigs map[string]string
	metadata   map[string]ConfigMetadata
}

var _ ConfigStore = (*memConfigStore)(nil)

// NewMemStore creates a ConfigStore backed by in-memory maps (for use in tests).
func NewMemStore() ConfigStore {
	return &memConfigStore{
		rawConfigs: make(map[string]string),
		metadata:   make(map[string]ConfigMetadata),
	}
}

// Close is a no-op for the in-memory store.
func (m *memConfigStore) Close(_ context.Context) error {
	return nil
}

// StoreConfig stores a device configuration, deduplicating against the latest stored config for the same device+type.
func (m *memConfigStore) StoreConfig(deviceID string, configType ConfigType, rawConfig string) (string, error) {
	rawHash := hashConfig(rawConfig)
	now := time.Now().Unix()

	m.lock.Lock()
	defer m.lock.Unlock()

	if existingID := m.findLatestMatch(deviceID, configType, rawHash); existingID != "" {
		return existingID, nil
	}

	configUUID := uuid.New().String()
	m.rawConfigs[configUUID] = rawConfig

	m.metadata[configUUID] = ConfigMetadata{
		ConfigUUID:     configUUID,
		DeviceID:       deviceID,
		ConfigType:     configType,
		CapturedAt:     now,
		LastAccessedAt: now,
		RawHash:        rawHash,
		AgentVersion:   version.AgentVersion,
	}

	return configUUID, nil
}

// findLatestMatch returns the UUID of the latest stored config for the given device+type if its hash matches.
// Must be called with m.lock held.
func (m *memConfigStore) findLatestMatch(deviceID string, configType ConfigType, rawHash string) string {
	var latest *ConfigMetadata
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
func (m *memConfigStore) CheckDuplicate(deviceID string, configType ConfigType, rawHash string) (string, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.findLatestMatch(deviceID, configType, rawHash), nil
}

// GetConfig retrieves all data for a config by UUID.
func (m *memConfigStore) GetConfig(configUUID string) (string, *ConfigMetadata, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	rawConfig, ok := m.rawConfigs[configUUID]
	if !ok {
		return "", nil, fmt.Errorf("raw config not found for UUID: %s", configUUID)
	}

	meta := m.metadata[configUUID]

	return rawConfig, &meta, nil
}
