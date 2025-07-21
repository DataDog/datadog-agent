// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package mock contains mock used to test the inventorysoftware component
package mock

import (
	"context"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/pkg/inventory/software"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/stretchr/testify/mock"
)

// SysProbeClient implements SysProbeClient for testing.
type SysProbeClient struct {
	mock.Mock
}

func (m *SysProbeClient) GetCheck(module types.ModuleName) ([]software.Entry, error) {
	args := m.Called(module)
	return args.Get(0).([]software.Entry), args.Error(1)
}

// Hostname implements hostnameinterface.Component for testing
type Hostname struct{}

func (m *Hostname) GetWithProvider(_ context.Context) (hostnameinterface.Data, error) {
	return hostnameinterface.Data{
		Hostname: "test-hostname",
		Provider: "test-provider",
	}, nil
}

func (m *Hostname) GetSafe(_ context.Context) string {
	return "test-hostname"
}

func (m *Hostname) Get(_ context.Context) (string, error) {
	return "test-hostname", nil
}
