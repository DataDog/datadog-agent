// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

// Package otlp fetch information from otlp collector
package otlp

import (
	"github.com/DataDog/datadog-agent/comp/otelcol/collector"
	otlpcollector "github.com/DataDog/datadog-agent/comp/otelcol/otlp"
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

// PopulateStatus populates stats with otlp information.
// TODO: (components): remove this logic when this package is migrated.
func PopulateStatus(status map[string]interface{}) {
	if otlpcollector.IsDisplayed() {
		otlpStatus := make(map[string]interface{})
		otlpIsEnabled := otlpcollector.IsEnabled(config.Datadog)
		var otlpCollectorStatus otlpcollector.CollectorStatus
		if otlpIsEnabled && otlpCollector != nil {
			otlpCollectorStatus = otlpCollector.Status()
		} else {
			otlpCollectorStatus = otlpcollector.CollectorStatus{Status: "Not running", ErrorMessage: ""}
		}
		otlpStatus["otlpStatus"] = otlpIsEnabled
		otlpStatus["otlpCollectorStatus"] = otlpCollectorStatus.Status
		otlpStatus["otlpCollectorStatusErr"] = otlpCollectorStatus.ErrorMessage

		status["otlp"] = otlpStatus
	}
}
