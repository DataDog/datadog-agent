// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package eventplatformreceiverimpl

import (
	eprinterface "github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// MockModule defines the fx options for the mocked component
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock))
}

// MockEventPlatformReceiver is the mocked struct that implements the eventplatformreceiver interface
type MockEventPlatformReceiver struct{}

// SetEnabled is a mocked method on the component
func (epr *MockEventPlatformReceiver) SetEnabled(e bool) bool {
	return e
}

// IsEnabled is a mocked method on the component
func (epr *MockEventPlatformReceiver) IsEnabled() bool {
	return true
}

// HandleMessage is a mocked method on the component
func (epr *MockEventPlatformReceiver) HandleMessage(_ *message.Message, _ []byte, _ string) {
}

// Filter is a mocked method on the component that returns a string channel
func (epr *MockEventPlatformReceiver) Filter(_ *diagnostic.Filters, _ <-chan struct{}) <-chan string {
	c := make(chan string)
	return c
}

// newMock returns the mocked eventplatformreceiver struct
func newMock() eprinterface.Component {
	return &MockEventPlatformReceiver{}
}
