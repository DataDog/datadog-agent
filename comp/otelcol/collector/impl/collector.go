// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

// Package collectorimpl provides the implementation of the collector component for OTel Agent
package collectorimpl

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/otelcol"

	corelog "github.com/DataDog/datadog-agent/comp/core/log"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	collectorcontrib "github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/def"
	collectordef "github.com/DataDog/datadog-agent/comp/otelcol/collector/def"
	"github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/datadogexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/infraattributesprocessor"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	zapAgent "github.com/DataDog/datadog-agent/pkg/util/log/zap"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type dependencies struct {
	// Lc specifies the compdef lifecycle settings, used for appending startup
	// and shutdown hooks.
	Lc compdef.Lifecycle

	// Log specifies the logging component.
	Log              corelog.Component
	Provider         otelcol.ConfigProvider
	CollectorContrib collectorcontrib.Component
	Serializer       serializer.MetricSerializer
	LogsAgent        optional.Option[logsagentpipeline.Component]
	SourceProvider   serializerexporter.SourceProviderFunc
}

type collector struct {
	deps dependencies
	set  otelcol.CollectorSettings
	col  *otelcol.Collector
}

// New returns a new instance of the collector component.
func New(deps dependencies) (collectordef.Component, error) {
	// Replace default core to use Agent logger
	options := []zap.Option{
		zap.WrapCore(func(zapcore.Core) zapcore.Core {
			return zapAgent.NewZapCore()
		}),
	}
	set := otelcol.CollectorSettings{
		BuildInfo: component.BuildInfo{
			Version:     "0.0.1",
			Command:     "otel-agent",
			Description: "Datadog Agent OpenTelemetry Collector Distribution",
		},
		LoggingOptions: options,
		Factories: func() (otelcol.Factories, error) {
			factories, err := deps.CollectorContrib.OTelComponentFactories()
			if err != nil {
				return otelcol.Factories{}, err
			}
			if v, ok := deps.LogsAgent.Get(); ok {
				factories.Exporters[datadogexporter.Type] = datadogexporter.NewFactory(deps.Serializer, v, deps.SourceProvider)
			} else {
				factories.Exporters[datadogexporter.Type] = datadogexporter.NewFactory(deps.Serializer, nil, deps.SourceProvider)
			}
			factories.Processors[infraattributesprocessor.Type] = infraattributesprocessor.NewFactory()
			return factories, nil
		},
		ConfigProvider: deps.Provider,
	}
	col, err := otelcol.NewCollector(set)
	if err != nil {
		return nil, err
	}
	c := &collector{
		deps: deps,
		set:  set,
		col:  col,
	}

	deps.Lc.Append(compdef.Hook{
		OnStart: c.start,
		OnStop:  c.stop,
	})
	return c, nil
}

func (c *collector) start(context.Context) error {
	go func() {
		if err := c.col.Run(context.Background()); err != nil {
			c.deps.Log.Errorf("Error running the collector pipeline: %v", err)
		}
	}()
	return nil
}

func (c *collector) stop(context.Context) error {
	c.col.Shutdown()
	return nil
}

func (c *collector) Status() otlp.CollectorStatus {
	return otlp.CollectorStatus{
		Status: c.col.GetState().String(),
	}
}
