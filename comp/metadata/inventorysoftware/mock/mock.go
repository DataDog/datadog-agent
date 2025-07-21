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
// This mock provides a testable implementation of the System Probe client
// interface, allowing tests to control the behavior of software inventory
// collection without requiring a real System Probe connection.
type SysProbeClient struct {
	mock.Mock
}

// GetCheck implements the SysProbeClient interface for testing.
// This method allows tests to specify expected calls and return values
// for software inventory collection, enabling comprehensive test coverage
// of the inventory software component.
func (m *SysProbeClient) GetCheck(module types.ModuleName) ([]software.Entry, error) {
	args := m.Called(module)
	return args.Get(0).([]software.Entry), args.Error(1)
}

// Hostname implements hostnameinterface.Component for testing.
// This mock provides a consistent hostname for testing purposes,
// ensuring that tests have predictable hostname values without
// depending on the actual system hostname.
type Hostname struct{}

// GetWithProvider returns test hostname data with provider information.
// This method provides a complete hostname data structure for testing,
// including both the hostname string and the provider information.
func (m *Hostname) GetWithProvider(_ context.Context) (hostnameinterface.Data, error) {
	return hostnameinterface.Data{
		Hostname: "test-hostname",
		Provider: "test-provider",
	}, nil
}

// GetSafe returns a safe hostname string for testing.
// This method provides a fallback hostname value that can be used
// when the primary hostname retrieval fails or is not available.
func (m *Hostname) GetSafe(_ context.Context) string {
	return "test-hostname"
}

// Get returns the test hostname with error handling.
// This method provides the standard hostname retrieval interface
// for testing, returning a consistent test hostname value.
func (m *Hostname) Get(_ context.Context) (string, error) {
	return "test-hostname", nil
}
