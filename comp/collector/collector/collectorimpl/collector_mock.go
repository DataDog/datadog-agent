// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package collectorimpl provides the implementation of the collector component.
package collectorimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Supply(MockParams{}),
		fx.Provide(newMock),
		fx.Provide(func(collector collector.Component) optional.Option[collector.Component] {
			return optional.NewOption(collector)
		}),
	)
}

// MockParams defines the parameters for the mock component.
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

// Start begins the collector's operation.  The scheduler will not run any checks until this has been called.
func (c *mockimpl) Start() {
}

// Stop halts any component involved in running a Check
func (c *mockimpl) Stop() {
}

// RunCheck sends a Check in the execution queue
func (c *mockimpl) RunCheck(_ check.Check) (checkid.ID, error) {
	return checkid.ID(""), nil
}

// StopCheck halts a check and remove the instance
func (c *mockimpl) StopCheck(_ checkid.ID) error {
	return nil
}

// MapOverChecks call the callback with the list of checks locked.
func (c *mockimpl) MapOverChecks(cb func([]check.Info)) {
	cb(c.checksInfo)
}

// GetChecks copies checks
func (c *mockimpl) GetChecks() []check.Check {
	return nil
}

// GetAllInstanceIDs returns the ID's of all instances of a check
func (c *mockimpl) GetAllInstanceIDs(_ string) []checkid.ID {
	return nil
}

// ReloadAllCheckInstances completely restarts a check with a new configuration
func (c *mockimpl) ReloadAllCheckInstances(_ string, _ []check.Check) ([]checkid.ID, error) {
	return []checkid.ID{checkid.ID("")}, nil
}

// AddEventReceiver adds a callback to the collector to be called each time a check is added or removed.
func (c *mockimpl) AddEventReceiver(_ collector.EventReceiver) {}
