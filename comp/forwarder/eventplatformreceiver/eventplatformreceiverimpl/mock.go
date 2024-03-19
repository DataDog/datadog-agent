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

func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock))
}

type MockEventPlatformReceiver struct{}

func (epr *MockEventPlatformReceiver) SetEnabled(e bool) bool {
	return e
}

func (epr *MockEventPlatformReceiver) IsEnabled() bool {
	return true
}

func (epr *MockEventPlatformReceiver) HandleMessage(m *message.Message, rendered []byte, eventType string) {
}

func (epr *MockEventPlatformReceiver) Filter(filters *diagnostic.Filters, done <-chan struct{}) <-chan string {
	c := make(chan string)
	return c
}

func newMock() eprinterface.Component {
	return &MockEventPlatformReceiver{}
}
