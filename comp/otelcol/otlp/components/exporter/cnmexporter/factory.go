// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package cnmexporter implements an OTel metrics exporter that reconstructs
// Datadog-native CollectorConnections protobuf payloads from CNM receiver metrics
// and submits them to the Datadog backend via the connections forwarder.
package cnmexporter

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	connectionsforwarder "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/def"
)

// NewFactory creates an OTel exporter factory for the CNM exporter in standalone mode.
func NewFactory() exporter.Factory {
	return exporter.NewFactory(
		component.MustNewType("datadog_cnm"),
		createDefaultConfig,
		exporter.WithMetrics(createStandaloneMetricsExporter, component.StabilityLevelAlpha),
	)
}

// NewFactoryForAgent creates an OTel exporter factory for the CNM exporter in Agent Core mode.
func NewFactoryForAgent(
	forwarder connectionsforwarder.Component,
	tagger tagger.Component,
	hostname hostname.Component,
	config coreconfig.Component,
	log log.Component,
) exporter.Factory {
	f := &cnmExporterFactory{
		forwarder: forwarder,
		tagger:    tagger,
		hostname:  hostname,
		config:    config,
		log:       log,
	}
	return exporter.NewFactory(
		component.MustNewType("datadog_cnm"),
		createDefaultConfig,
		exporter.WithMetrics(f.createAgentMetricsExporter, component.StabilityLevelAlpha),
	)
}

// cnmExporterFactory holds Agent Core dependencies for creating CNM exporters.
type cnmExporterFactory struct {
	forwarder connectionsforwarder.Component
	tagger    tagger.Component
	hostname  hostname.Component
	config    coreconfig.Component
	log       log.Component
}

func createStandaloneMetricsExporter(
	ctx context.Context,
	settings exporter.Settings,
	baseCfg component.Config,
) (exporter.Metrics, error) {
	cfg := baseCfg.(*Config)
	exp := newCNMExporter(cfg, settings.Logger, nil)
	return exporterhelper.NewMetrics(ctx, settings, baseCfg, exp.ConsumeMetrics,
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: false}),
		exporterhelper.WithShutdown(exp.Shutdown),
	)
}

func (f *cnmExporterFactory) createAgentMetricsExporter(
	ctx context.Context,
	settings exporter.Settings,
	baseCfg component.Config,
) (exporter.Metrics, error) {
	cfg := baseCfg.(*Config)

	h := ""
	if f.hostname != nil {
		if resolved, err := f.hostname.Get(ctx); err == nil {
			h = resolved
		}
	}

	exp := newCNMExporter(cfg, settings.Logger, f.forwarder)
	exp.hostname = h
	exp.tagger = f.tagger

	return exporterhelper.NewMetrics(ctx, settings, baseCfg, exp.ConsumeMetrics,
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: false}),
		exporterhelper.WithShutdown(exp.Shutdown),
	)
}
