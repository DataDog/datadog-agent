// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package noop implements the event platform receiver component.
package noop

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type Noop struct{}

func (Noop) SetEnabled(e bool) bool {
	return false
}
func (Noop) IsEnabled() bool {
	return false
}
func (Noop) HandleMessage(_ *message.Message, _ []byte, _ string) {

}
func (Noop) Filter(_ *diagnostic.Filters, _ <-chan struct{}) <-chan string {
	return nil
}

func new() eventplatformreceiver.Component {
	return Noop{}
}

func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(new),
	)
}
