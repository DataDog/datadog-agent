// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the collector component.
package mock

import (
	"go.uber.org/fx"

	collector "github.com/DataDog/datadog-agent/comp/collector/collector/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockParams defines the parameters for the mock collector component.
// It is designed to be used with fx.Replace:
//
//	fx.Replace(collectormock.MockParams{ChecksInfo: someChecksInfo})
type MockParams struct {
	ChecksInfo []check.Info
}

type mockDependencies struct {
	fx.In

	Params MockParams
}

type mockimpl struct {
	collector.Component

	checksInfo []check.Info
}

func newMock(deps mockDependencies) collector.Component {
	return &mockimpl{
		checksInfo: deps.Params.ChecksInfo,
	}
}

// Start begins the collector's operation.
func (c *mockimpl) Start() {}

// Stop halts any component involved in running a Check.
func (c *mockimpl) Stop() {}

// RunCheck sends a Check in the execution queue.
func (c *mockimpl) RunCheck(_ check.Check) (checkid.ID, error) {
	return checkid.ID(""), nil
}

// StopCheck halts a check and remove the instance.
func (c *mockimpl) StopCheck(_ checkid.ID) error {
	return nil
}

// MapOverChecks calls the callback with the list of checks locked.
func (c *mockimpl) MapOverChecks(cb func([]check.Info)) {
	cb(c.checksInfo)
}

// GetChecks copies checks.
func (c *mockimpl) GetChecks() []check.Check {
	return nil
}

// ReloadAllCheckInstances completely restarts a check with a new configuration.
func (c *mockimpl) ReloadAllCheckInstances(_ string, _ []check.Check) ([]checkid.ID, error) {
	return []checkid.ID{checkid.ID("")}, nil
}

// AddEventReceiver adds a callback to the collector to be called each time a check is added or removed.
func (c *mockimpl) AddEventReceiver(_ collector.EventReceiver) {}

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock),
		fx.Supply(MockParams{}),
		fxutil.ProvideOptional[collector.Component](),
	)
}
