// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package fetchonlyimpl

import (
	"crypto/tls"

	"go.uber.org/fx"

	authtokeninterface "github.com/DataDog/datadog-agent/comp/api/authtoken"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock),
	)
}

// MockFetchOnly is a mock for fetch only authtoken
type MockFetchOnly struct{}

// Get is a mock of the fetchonly Get function
func (fc *MockFetchOnly) Get() string {
	return "a string"
}

// GetTLSClientConfig is a mock of the fetchonly GetTLSClientConfig function
func (fc *MockFetchOnly) GetTLSClientConfig() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true,
	}
}

// GetTLSServerConfig is a mock of the fetchonly GetTLSServerConfig function
func (fc *MockFetchOnly) GetTLSServerConfig() *tls.Config {
	return &tls.Config{}
}

// NewMock returns a new fetch only authtoken mock
func newMock() authtokeninterface.Component {
	return &MockFetchOnly{}
}
