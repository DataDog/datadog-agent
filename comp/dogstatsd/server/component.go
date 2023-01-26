// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package server implements a component to run the dogstatsd server
package server

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// team: Agent Metrics Logs

// Component is the component type.
type Component interface {

	// Start starts the dogstatsd server
	Start(demultiplexer aggregator.Demultiplexer) error
	// Stop stops the dogstatsd server
	Stop()

	// Capture starts a new dogstatsd traffic capture
	Capture(p string, d time.Duration, compressed bool) error

	// IsCaputreOngoing returns true if a capture is ongoing
	IsCaputreOngoing() bool

	// GetCapturePath returns the capture destination path
	GetCapturePath() (string, error)

	// GetJSONDebugStats returns a json representation of debug stats
	GetJSONDebugStats() ([]byte, error)

	// GetJSONDebugStats gets the DsdServerDebug instance which provides metric stats
	GetDebug() *dogstatsd.DsdServerDebug

	// EnableMetricsStats enables metric stats tracking
	EnableMetricsStats()

	// DisableMetricsStats disables metric stats tracking
	DisableMetricsStats()

	// UdsListenerRunning returns true if the uds listener is running
	UdsListenerRunning() bool
}

// Mock implements mock-specific methods.
type Mock interface {
	Component
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newServer),
)

// TODO: Create a mock version once dogstatsd server is migrated
// MockModule defines the fx options for the mock component.
// var MockModule = fxutil.Component(
// fx.Provide(newMock),
// )
