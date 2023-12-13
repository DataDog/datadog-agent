// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package collector

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/stretchr/testify/mock"
)

// MockCollector mock the Collector interface
type MockCollector struct {
	mock.Mock
	checksInfo []check.Info
}

// NewMock returns a mock collector.
//
// 'mockChecksInfo' will be used as argument for the callback when 'MapOverChecks' is called.
func NewMock(mockChecksInfo []check.Info) *MockCollector {
	return &MockCollector{
		checksInfo: mockChecksInfo,
	}
}

// Start begins the collector's operation.  The scheduler will not run any checks until this has been called.
func (c *MockCollector) Start() {
	c.Called()
}

// Stop halts any component involved in running a Check
func (c *MockCollector) Stop() {
	c.Called()
}

// RunCheck sends a Check in the execution queue
func (c *MockCollector) RunCheck(inner check.Check) (checkid.ID, error) {
	args := c.Called(inner)
	return args.Get(0).(checkid.ID), args.Error(1)
}

// StopCheck halts a check and remove the instance
func (c *MockCollector) StopCheck(id checkid.ID) error {
	args := c.Called(id)
	return args.Error(0)
}

// MapOverChecks call the callback with the list of checks locked.
func (c *MockCollector) MapOverChecks(cb func([]check.Info)) {
	c.Called(cb)
	cb(c.checksInfo)
}

// GetChecks copies checks
func (c *MockCollector) GetChecks() []check.Check {
	args := c.Called()
	return args.Get(0).([]check.Check)
}

// GetAllInstanceIDs returns the ID's of all instances of a check
func (c *MockCollector) GetAllInstanceIDs(checkName string) []checkid.ID {
	args := c.Called(checkName)
	return args.Get(0).([]checkid.ID)
}

// ReloadAllCheckInstances completely restarts a check with a new configuration
func (c *MockCollector) ReloadAllCheckInstances(name string, newInstances []check.Check) ([]checkid.ID, error) {
	args := c.Called(name, newInstances)
	return args.Get(0).([]checkid.ID), args.Error(1)
}

// AddEventReceiver adds a callback to the collector to be called each time a check is added or removed.
func (c *MockCollector) AddEventReceiver(cb EventReceiver) {
	c.Called(cb)
}
