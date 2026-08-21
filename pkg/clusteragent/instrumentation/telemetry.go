// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package instrumentation

import (
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
)

const (
	telemetrySubsystem = "instrumentation_controller"
	statusSuccess      = "success"
	statusError        = "error"
)

type telemetryRecorder interface {
	recordReconciliation(section string, success bool)
	setResources(total int)
}

type controllerTelemetry struct {
	resources       telemetry.Gauge
	reconciliations telemetry.Counter
}

var defaultControllerTelemetry telemetryRecorder = &controllerTelemetry{
	resources: telemetryimpl.GetCompatComponent().NewGaugeWithOpts(
		telemetrySubsystem,
		"resources",
		nil,
		"Number of DatadogInstrumentation resources tracked by the controller.",
		telemetry.DefaultOptions,
	),
	reconciliations: telemetryimpl.GetCompatComponent().NewCounterWithOpts(
		telemetrySubsystem,
		"reconciliations",
		[]string{"section", "status"},
		"Number of DatadogInstrumentation section reconciliation attempts by outcome.",
		telemetry.DefaultOptions,
	),
}

func (t *controllerTelemetry) recordReconciliation(section string, success bool) {
	result := statusError
	if success {
		result = statusSuccess
	}
	t.reconciliations.Inc(section, result)
}

func (t *controllerTelemetry) setResources(total int) {
	t.resources.Set(float64(total))
}
