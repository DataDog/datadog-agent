// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package inventorysoftware

import (
	"context"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	softwareinventory "github.com/DataDog/datadog-agent/pkg/inventory/software"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/stretchr/testify/mock"
)

// mockSysProbeClient implements SysProbeClient for testing.
type mockSysProbeClient struct {
	mock.Mock
}

func (m *mockSysProbeClient) GetCheck(module types.ModuleName) ([]softwareinventory.SoftwareEntry, error) {
	args := m.Called(module)
	return args.Get(0).([]softwareinventory.SoftwareEntry), args.Error(1)
}

// mockHostname implements hostnameinterface.Component for testing
type mockHostname struct{}

func (m *mockHostname) GetWithProvider(_ context.Context) (hostnameinterface.Data, error) {
	return hostnameinterface.Data{
		Hostname: "test-hostname",
		Provider: "test-provider",
	}, nil
}

func (m *mockHostname) GetSafe(_ context.Context) string {
	return "test-hostname"
}

func (m *mockHostname) Get(_ context.Context) (string, error) {
	return "test-hostname", nil
}
