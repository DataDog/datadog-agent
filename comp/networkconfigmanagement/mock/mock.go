// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ncm

// Package mock provides a mock for the networkconfigmanagement component
package mock

import (
	"sync"
	"testing"
	"time"

	"go.uber.org/fx"

	networkconfigmanagement "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/def"
	ncmstore "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/store"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type mockNetworkConfigManagement struct {
	store ncmstore.ConfigStore

	inventoryLock         sync.Mutex
	lastInventoryReportAt time.Time
}

// Mock returns a networkconfigmanagement.Component backed by an in-memory store.
func Mock(_ *testing.T) networkconfigmanagement.Component {
	return &mockNetworkConfigManagement{store: ncmstore.NewMemStore()}
}

// MockWithStore returns a networkconfigmanagement.Component backed by the
// provided store. Useful for tests that need a memstore with custom options
// (e.g. deterministic clock or UUID generator) so inventory output is
// predictable.
func MockWithStore(_ *testing.T, store ncmstore.ConfigStore) networkconfigmanagement.Component {
	return &mockNetworkConfigManagement{store: store}
}

// MockModule provides the mock as an fx module.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(func() networkconfigmanagement.Component {
			return &mockNetworkConfigManagement{store: ncmstore.NewMemStore()}
		}),
	)
}

func (m *mockNetworkConfigManagement) GetConfigStore() ncmstore.ConfigStore {
	return m.store
}

func (m *mockNetworkConfigManagement) ShouldSendInventoryReport(hasNewConfigs bool, maxInterval time.Duration, now time.Time) bool {
	m.inventoryLock.Lock()
	defer m.inventoryLock.Unlock()
	if !hasNewConfigs && now.Sub(m.lastInventoryReportAt) < maxInterval {
		return false
	}
	m.lastInventoryReportAt = now
	return true
}
