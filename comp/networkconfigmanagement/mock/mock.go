// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package mock provides a mock for the networkconfigmanagement component
package mock

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	networkconfigmanagement "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/config"
	ncmstore "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/store"
)

type mockNetworkConfigManagement struct {
	store                 ncmstore.ConfigStore
	devices               map[string]*config.DeviceInstance
	inventoryLock         sync.Mutex
	lastInventoryReportAt time.Time
}

// ReportConfig implements [networkconfigmanagement.Component].
func (m *mockNetworkConfigManagement) ReportConfig(deviceID string) error {
	if _, ok := m.devices[deviceID]; ok {
		return nil
	}
	return fmt.Errorf("unrecognized device %s", deviceID)
}

// ReportConfig implements [networkconfigmanagement.Component].
func (m *mockNetworkConfigManagement) ReportConfigWithSender(deviceID string, _ sender.Sender) error {
	if _, ok := m.devices[deviceID]; ok {
		return nil
	}
	return fmt.Errorf("unrecognized device %s", deviceID)
}

// RegisterDevice implements [networkconfigmanagement.Component].
func (m *mockNetworkConfigManagement) RegisterDevice(device *config.DeviceInstance) error {
	m.devices[device.DeviceID()] = device
	return nil
}

// RollbackConfig implements [networkconfigmanagement.Component].
func (m *mockNetworkConfigManagement) RollbackConfig(_ string, _ string, _ string) error {
	return errors.New("TODO unimplemented")
}

// SetMaxReportInterval implements [networkconfigmanagement.Component].
func (m *mockNetworkConfigManagement) SetMaxReportInterval(_ time.Duration) error {
	return nil
}

// Mock returns a networkconfigmanagement.Component backed by an in-memory store.
func Mock(_ *testing.T) networkconfigmanagement.Component {
	return &mockNetworkConfigManagement{
		store:   ncmstore.NewMemStore(),
		devices: make(map[string]*config.DeviceInstance),
	}
}

// MockWithStore returns a networkconfigmanagement.Component backed by the
// provided store. Useful for tests that need a memstore with custom options
// (e.g. deterministic clock or UUID generator) so inventory output is
// predictable.
func MockWithStore(_ *testing.T, store ncmstore.ConfigStore) networkconfigmanagement.Component {
	return &mockNetworkConfigManagement{store: store}
}

func (m *mockNetworkConfigManagement) GetConfigStore() ncmstore.ConfigStore {
	return m.store
}

func (m *mockNetworkConfigManagement) MeetsInventoryReportRequirements(hasNewConfigs bool, maxInterval time.Duration, now time.Time) bool {
	m.inventoryLock.Lock()
	defer m.inventoryLock.Unlock()
	if !hasNewConfigs && now.Sub(m.lastInventoryReportAt) < maxInterval {
		return false
	}
	m.lastInventoryReportAt = now
	return true
}

func (m *mockNetworkConfigManagement) MarkInventoryReportSent(now time.Time) {
	m.inventoryLock.Lock()
	defer m.inventoryLock.Unlock()
	m.lastInventoryReportAt = now
}
