// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

package status

import (
	"github.com/DataDog/datadog-agent/comp/otelcol/collector"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// otlpCollector holds an instance of any running collector.
// It is assigned in cmd/agent/subcommands/run.go.(startAgent)
// Will be nil otherwise!
// TODO: (components) remove once this package is migrated to components.
var otlpCollector collector.Component

// SetOtelCollector registers the active OTLP Collector for status queries.
// Warning, this function is not synchronised.
// TODO: (components): remove this logic when this package is migrated.
func SetOtelCollector(c collector.Component) {
	otlpCollector = c
}

// GetOTLPStatus parses the otlp pipeline and its collector info to be sent to the frontend
func GetOTLPStatus() map[string]interface{} {
	status := make(map[string]interface{})
	otlpIsEnabled := otlp.IsEnabled(config.Datadog)
	var otlpCollectorStatus otlp.CollectorStatus
	if otlpIsEnabled && otlpCollector != nil {
		otlpCollectorStatus = otlpCollector.Status()
	} else {
		otlpCollectorStatus = otlp.CollectorStatus{Status: "Not running", ErrorMessage: ""}
	}
	status["otlpStatus"] = otlpIsEnabled
	status["otlpCollectorStatus"] = otlpCollectorStatus.Status
	status["otlpCollectorStatusErr"] = otlpCollectorStatus.ErrorMessage
	return status
}
