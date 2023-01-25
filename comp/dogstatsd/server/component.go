// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package runner implements a component to run data collection checks in the Process Agent.
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
	Start(demultiplexer aggregator.Demultiplexer)
	Stop()

	Capture(p string, d time.Duration, compressed bool) error
	IsCaputreOngoing() bool
	GetCapturePath() (string, error)

	GetJSONDebugStats() ([]byte, error)
	GetDebug() *dogstatsd.DsdServerDebug

	EnableMetricsStats()
	DisableMetricsStats()
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

// MockModule defines the fx options for the mock component.
var MockModule = fxutil.Component(
// fx.Provide(newMock),
)
