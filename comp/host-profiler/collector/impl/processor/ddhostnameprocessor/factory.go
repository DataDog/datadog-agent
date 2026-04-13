// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package ddhostnameprocessor

import (
	"context"
	"sync"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/hostmetadata"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/xconsumer"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"
	"go.opentelemetry.io/collector/processor/processorhelper/xprocessorhelper"
	"go.opentelemetry.io/collector/processor/xprocessor"
	"go.uber.org/zap"
)

var processorCapabilities = consumer.Capabilities{MutatesData: true}

func createDefaultConfig() component.Config {
	return &Config{}
}

// NewFactory returns a new factory for the ddhostname processor.
func NewFactory() processor.Factory {
	f := &factory{}
	return xprocessor.NewFactory(
		component.MustNewType("ddhostname"),
		createDefaultConfig,
		xprocessor.WithMetrics(f.createMetricsProcessor, component.StabilityLevelAlpha),
		xprocessor.WithLogs(f.createLogsProcessor, component.StabilityLevelAlpha),
		xprocessor.WithTraces(f.createTracesProcessor, component.StabilityLevelAlpha),
		xprocessor.WithProfiles(f.createProfilesProcessor, component.StabilityLevelAlpha),
	)
}

type factory struct {
	host string
	once sync.Once
}

func (f *factory) resolveHost(ctx context.Context, set processor.Settings) string {
	f.once.Do(func() {
		sourceProvider, err := hostmetadata.GetSourceProvider(set.TelemetrySettings, "", 0)
		if err == nil {
			source, err := sourceProvider.Source(ctx)
			if err == nil && source.Kind == "host" {
				f.host = source.Identifier
			}
		}

		if f.host == "" {
			set.Logger.Warn("Could not resolve host for standalone mode")
		} else {
			set.Logger.Info("Resolved host for standalone mode", zap.String("hostname", f.host))
		}
	})
	return f.host
}

func (f *factory) createMetricsProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	nextConsumer consumer.Metrics,
) (processor.Metrics, error) {
	p := &ddhostnameProcessor{host: f.resolveHost(ctx, set)}
	return processorhelper.NewMetrics(ctx, set, cfg, nextConsumer,
		p.processMetrics,
		processorhelper.WithCapabilities(processorCapabilities))
}

func (f *factory) createLogsProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	nextConsumer consumer.Logs,
) (processor.Logs, error) {
	p := &ddhostnameProcessor{host: f.resolveHost(ctx, set)}
	return processorhelper.NewLogs(ctx, set, cfg, nextConsumer,
		p.processLogs,
		processorhelper.WithCapabilities(processorCapabilities))
}

func (f *factory) createTracesProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	nextConsumer consumer.Traces,
) (processor.Traces, error) {
	p := &ddhostnameProcessor{host: f.resolveHost(ctx, set)}
	return processorhelper.NewTraces(ctx, set, cfg, nextConsumer,
		p.processTraces,
		processorhelper.WithCapabilities(processorCapabilities))
}

func (f *factory) createProfilesProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	nextConsumer xconsumer.Profiles,
) (xprocessor.Profiles, error) {
	p := &ddhostnameProcessor{host: f.resolveHost(ctx, set)}
	return xprocessorhelper.NewProfiles(ctx, set, cfg, nextConsumer,
		p.processProfiles,
		xprocessorhelper.WithCapabilities(processorCapabilities))
}
