// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package eventplatformreceiverimpl implements the event platform receiver component.
package eventplatformreceiverimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewReceiver),
	)
}

// NewReceiver returns a new event platform receiver.
func NewReceiver(hostname hostnameinterface.Component) eventplatformreceiver.Component {
	return diagnostic.NewBufferedMessageReceiver(&epFormatter{}, hostname)
}
