// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package server implements a component to run the dogstatsd capture/replay
//
//nolint:revive // TODO(AML) Fix revive linter
package replay

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-metrics-logs

// Component is the component type.
type Component interface {
	// TODO: (components) we should remove the configure method once Dogstatsd's lifecycle is managed by FX (start/stop)
	// Because Dogstatsd can fail at runtime, we can't yet rely on fx to configure and start up the replay feature
	// since it has no way of knowing if dogstatsd started successfully.
	Configure() error

	// IsOngoing returns whether a capture is ongoing for this TrafficCapture instance.
	IsOngoing() bool

	// Start starts a TrafficCapture and returns an error in the event of an issue.
	Start(p string, d time.Duration, compressed bool) (string, error)

	// Stop stops an ongoing TrafficCapture.
	Stop()

	// TODO: (components) pool manager should be injected as a component in the future.
	// RegisterSharedPoolManager registers the shared pool manager with the TrafficCapture.
	RegisterSharedPoolManager(p *packets.PoolManager) error

	// TODO: (components) pool manager should be injected as a component in the future.
	// RegisterOOBPoolManager registers the OOB shared pool manager with the TrafficCapture.f
	RegisterOOBPoolManager(p *packets.PoolManager) error

	// Enqueue enqueues a capture buffer so it's written to file.
	Enqueue(msg *CaptureBuffer) bool
}

// Mock implements mock-specific methods.
type Mock interface {
	Component
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	panic("not called")
}

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	panic("not called")
}
