// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
)

// SimpleEventMonitorModule defines a simple event monitor module
// Implement the consumer interface
type SimpleEventMonitorModule struct {
	Consumer SimpleEventConsumer
}

// NewSimpleEventConsumer returns a new simple event consumer
func NewSimpleEventMonitorModule(em *eventmonitor.EventMonitor) *SimpleEventMonitorModule {
	fc := &SimpleEventMonitorModule{}
	_ = em.AddEventConsumer(&fc.Consumer)
	return fc
}

// ID returns the ID of this module
// Implement the module interface
func (fc *SimpleEventMonitorModule) ID() string {
	return "simple_module"
}

// Start the module
// Implement the module interface
func (fc *SimpleEventMonitorModule) Start() error {
	return nil
}

// Stop the module
// Implement the module interface
func (fc *SimpleEventMonitorModule) Stop() {
}
