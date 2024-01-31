// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package server implements a component to run the dogstatsd server
package server

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// team: agent-metrics-logs

// Component is the component type.
type Component interface {

	// // Start starts the dogstatsd server
	// Start(demultiplexer aggregator.Demultiplexer) error

	// Stop stops the dogstatsd server
	Stop()

	// IsRunning returns true if the server is running
	IsRunning() bool

	// UdsListenerRunning returns true if the uds listener is running
	UdsListenerRunning() bool

	// ServerlessFlush flushes all the data to the aggregator to them send it to the Datadog intake.
	ServerlessFlush(time.Duration)

	// SetExtraTags sets extra tags. All metrics sent to the DogstatsD will be tagged with them.
	SetExtraTags(tags []string)

	// UDPLocalAddr returns the local address of the UDP statsd listener, if enabled.
	UDPLocalAddr() string
}

// Mock implements mock-specific methods.
type Mock interface {
	Component
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newServer))
}

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock))
}
