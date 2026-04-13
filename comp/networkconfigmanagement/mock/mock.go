// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package mock provides a mock for the networkconfigmanagement component
package mock

import (
	"testing"

	"go.uber.org/fx"

	networkconfigmanagement "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/def"
	ncmstore "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/store"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type mockNetworkConfigManagement struct {
	store ncmstore.ConfigStore
}

// Mock returns a networkconfigmanagement.Component backed by an in-memory store.
func Mock(_ *testing.T) networkconfigmanagement.Component {
	return &mockNetworkConfigManagement{store: ncmstore.NewMemStore()}
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
