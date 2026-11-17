// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package datadogexporter

import (
	datadogconfig "github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/datadogconfig"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/logsagentexporter"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/di"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter"
)

func diDecorator(cfg *datadogconfig.Config) logsagentexporter.ConsumerDecorator {
	return func(set exporter.Settings, ordinary consumer.ConsumeLogsFunc) (consumer.ConsumeLogsFunc, component.Component, error) {
		router, err := di.New(set.TelemetrySettings, di.Options{
			Enabled: cfg.DynamicInstrumentation.Enabled, Key: string(cfg.API.Key), Site: cfg.API.Site,
			Endpoint: cfg.DynamicInstrumentation.Endpoint, HTTP: cfg.ClientConfig,
		}, ordinary)
		if err != nil {
			return nil, nil, err
		}
		return router.ConsumeLogs, router, nil
	}
}
