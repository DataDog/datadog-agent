// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package telemetry

import (
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
)

// GetCompatComponent returns a component wrapping telemetry global variables.
//
// TODO: This is a compatibility layer for legacy code that hasn't migrated to FX.
// The primary consumer is pkg/network/tracer, which requires the full Prometheus
// interface (RegisterCollector/UnregisterCollector). Once pkg/network/tracer is
// migrated to request its telemetry dependency via FX, this function can be removed
// or simplified to return only the base telemetry.Component interface.
func GetCompatComponent() telemetryimpl.Component {
	return telemetryimpl.GetCompatComponent()
}
