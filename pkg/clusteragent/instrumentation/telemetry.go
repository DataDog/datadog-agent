// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package instrumentation

import telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"

const (
	telemetrySubsystem = "instrumentation_controller"
	statusSuccess      = "success"
	statusError        = "error"
)

type controllerTelemetry struct {
	resources       telemetry.Gauge
	reconciliations telemetry.Counter
}

func newControllerTelemetry(telemetryComp telemetry.Component) *controllerTelemetry {
	return &controllerTelemetry{
		resources: telemetryComp.NewGaugeWithOpts(
			telemetrySubsystem,
			"resources",
			nil,
			"Number of DatadogInstrumentation resources tracked by the controller.",
			telemetry.DefaultOptions,
		),
		reconciliations: telemetryComp.NewCounterWithOpts(
			telemetrySubsystem,
			"reconciliations",
			[]string{"section", "status"},
			"Number of DatadogInstrumentation section reconciliation attempts by outcome.",
			telemetry.DefaultOptions,
		),
	}
}

func (t *controllerTelemetry) recordReconciliation(section string, success bool) {
	result := statusError
	if success {
		result = statusSuccess
	}
	t.reconciliations.Inc(section, result)
}
