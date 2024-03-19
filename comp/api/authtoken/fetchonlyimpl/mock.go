// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package fetchonlyimpl

import (
	authtokeninterface "github.com/DataDog/datadog-agent/comp/api/authtoken"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock),
	)
}

// MockFlare is a mock of the
type MockFetchOnly struct{}

func (fc *MockFetchOnly) Get() string {
	return "a string"
}

// NewMock returns a new flare provider
func newMock() authtokeninterface.Component {
	return &MockFetchOnly{}
}
