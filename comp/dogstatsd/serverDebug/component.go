// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package serverDebug implements a component to run the dogstatsd server debug
package serverDebug

import (
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// team: agent-metrics-logs

// Component is the component type.
type Component interface {

	// StoreMetricStats stores stats on the given metric sample.
	StoreMetricStats(sample metrics.MetricSample)

	// IsDebugEnabled gets the DsdServerDebug instance which provides metric stats
	IsDebugEnabled() bool
	// SetMetricStatsEnabled enables or disables metric stats tracking
	SetMetricStatsEnabled(bool)

	// GetJSONDebugStats returns a json representation of debug stats
	GetJSONDebugStats() ([]byte, error)
}

// Mock implements mock-specific methods.
type Mock interface {
	Component
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newServerDebug),
)

// MockModule defines the fx options for the mock component.
var MockModule = fxutil.Component(
	fx.Provide(newMockServerDebug),
)
