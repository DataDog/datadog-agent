// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package flareimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/flare"
	finterface "github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewMock),
	)
}

// MockFlare is a mock of the
type MockFlare struct{}

func (fc *MockFlare) Create(profile flare.ProfileData, ipcError error) (string, error) {
	return "a string", nil
}

func (fc *MockFlare) Send(flarePath string, caseID string, email string, source helpers.FlareSource) (string, error) {
	return "a string", nil
}

// NewMock returns a new flare provider
func NewMock() finterface.Component {
	return &MockFlare{}
}
