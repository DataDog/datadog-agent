// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package mock provides a mock for the networkconfigmanagement component
package mock

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	networkconfigmanagement "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/config"
	ncmremote "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/remote"
	ncmstore "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/store"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
)

type mockNetworkConfigManagement struct {
	store   ncmstore.ConfigStore
	devices map[string]*config.DeviceInstance
}

// RollbackEndpointHandler implements [networkconfigmanagement.Component].
func (m *mockNetworkConfigManagement) RollbackEndpointHandler() http.HandlerFunc {
	panic("unimplemented")
}

// GetConfigEndpointHandler implements [networkconfigmanagement.Component].
func (m *mockNetworkConfigManagement) GetConfigEndpointHandler() http.HandlerFunc {
	panic("unimplemented")
}

// ReportConfig implements [networkconfigmanagement.Component].
func (m *mockNetworkConfigManagement) ReportConfig(_ context.Context, deviceID string, _ sender.Sender) error {
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
func (m *mockNetworkConfigManagement) RollbackConfig(_ context.Context, _, _, _ string) (*ncmremote.PushResult, types.RollbackError) {
	return nil, types.InternalError(errors.New("unimplemented"))
}

// SetMaxReportInterval implements [networkconfigmanagement.Component].
func (m *mockNetworkConfigManagement) SetMaxReportInterval(_ time.Duration) {}

// Mock returns a networkconfigmanagement.Component backed by an in-memory store.
func Mock(t *testing.T) networkconfigmanagement.Component {
	return MockWithStore(t, ncmstore.NewMemStore())
}

// MockWithStore returns a networkconfigmanagement.Component backed by the
// provided store. Useful for tests that need a memstore with custom options
// (e.g. deterministic clock or UUID generator) so inventory output is
// predictable.
func MockWithStore(_ *testing.T, store ncmstore.ConfigStore) networkconfigmanagement.Component {
	return &mockNetworkConfigManagement{
		store:   store,
		devices: make(map[string]*config.DeviceInstance),
	}
}
