// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverimpl

import (
	"github.com/DataDog/datadog-agent/pkg/remoteflags"
	"go.uber.org/atomic"
)

// extendedClientTelemetryPrefix is the metric name prefix for extended DogStatsD
// client telemetry. All metrics under this prefix are dropped by the agent unless
// the debug_dsd_client_queue_occupancy_ratio remote flag is enabled.
const extendedClientTelemetryPrefix = "datadog.dogstatsd.clientextended"

const extendedClientTelemetryFlagName remoteflags.FlagName = "debug_dsd_extended_client_telemetry"

type extendedClientTelemetryFlagHandler struct {
	enabled *atomic.Bool
}

func (h *extendedClientTelemetryFlagHandler) FlagName() remoteflags.FlagName {
	return extendedClientTelemetryFlagName
}

func (h *extendedClientTelemetryFlagHandler) OnChange(v remoteflags.FlagValue) error {
	h.enabled.Store(bool(v))
	return nil
}

func (h *extendedClientTelemetryFlagHandler) OnNoConfig() { h.enabled.Store(false) }

func (h *extendedClientTelemetryFlagHandler) SafeRecover(_ error, _ remoteflags.FlagValue) {
	h.enabled.Store(false)
}

func (h *extendedClientTelemetryFlagHandler) IsHealthy() bool { return true }
