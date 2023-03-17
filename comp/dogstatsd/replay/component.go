// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package server implements a component to run the dogstatsd capture/replay
package replay

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// team: agent-metrics-logs

// Component is the component type.
type Component interface {
	Configure() error
	IsOngoing() bool
	Start(p string, d time.Duration, compressed bool) (string, error)
	Stop()
	RegisterSharedPoolManager(p *packets.PoolManager) error
	RegisterOOBPoolManager(p *packets.PoolManager) error
	Enqueue(msg *CaptureBuffer) bool
}

// Mock implements mock-specific methods.
type Mock interface {
	Component
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newTrafficCapture),
)

// // MockModule defines the fx options for the mock component.
var MockModule = fxutil.Component(
	fx.Provide(newMockTrafficCapture),
)
