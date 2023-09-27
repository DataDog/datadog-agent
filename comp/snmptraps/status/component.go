// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package status exposes the expvars we use for status tracking to the
// component system.
package status

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// team: network-device-monitoring

// Component is the component type.
type Component interface {
	AddTrapsPackets(int64)
	GetTrapsPackets() int64
	AddTrapsPacketsAuthErrors(int64)
	GetTrapsPacketsAuthErrors() int64
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(new),
)

// MockModule defines a fake Component
var MockModule = fxutil.Component(
	fx.Provide(newMock),
)
