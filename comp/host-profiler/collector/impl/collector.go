// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package collectorimpl implements the collector component interface
package collectorimpl

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	compdef "github.com/DataDog/datadog-agent/comp/def"
	collector "github.com/DataDog/datadog-agent/comp/host-profiler/collector/def"
	zapAgent "github.com/DataDog/datadog-agent/pkg/util/log/zap"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/featuregate"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/sdk/metric"
)

// Params contains the parameters for the collector component.
type Params struct {
	uri              string
	GoRuntimeMetrics bool
}

// NewParams creates a new Params instance.
func NewParams(uri string, goRuntimeMetrics bool) Params {
	return Params{
		uri:              uri,
		GoRuntimeMetrics: goRuntimeMetrics,
	}
}

// Requires defines the dependencies for the collector component
type Requires struct {
	Params         Params
	ExtraFactories ExtraFactories
	Lc             compdef.Lifecycle
}

// Provides defines the output of the collector component.
type Provides struct {
	Comp collector.Component
}

type collectorImpl struct {
	collector *otelcol.Collector
}

// NewComponent creates a new collector component
func NewComponent(reqs Requires) (Provides, error) {
	// Enable profiles support (disabled by default)
	err := featuregate.GlobalRegistry().Set("service.profilesSupport", true)
	if err != nil {
		return Provides{}, err
	}

	settings, err := newCollectorSettings(reqs.Params.uri, reqs.ExtraFactories)
	if err != nil {
		return Provides{}, err
	}
	collector, err := otelcol.NewCollector(settings)
	if err != nil {
		return Provides{}, err
	}
	if reqs.Params.GoRuntimeMetrics {
		err = registerInstrumentation(reqs.Lc)
		if err != nil {
			return Provides{}, err
		}
	}
	provides := Provides{
		Comp: &collectorImpl{
			collector: collector,
		},
	}
	return provides, nil
}

func (c *collectorImpl) Run() error {
	return c.collector.Run(context.Background())
}

func newCollectorSettings(uri string, extraFactories ExtraFactories) (otelcol.CollectorSettings, error) {
	// Replace logger to use Agent logger
	options := []zap.Option{
		zap.WrapCore(func(zapcore.Core) zapcore.Core {
			return zapAgent.NewZapCore()
		}),
	}

	return otelcol.CollectorSettings{
		Factories:      createFactories(extraFactories),
		LoggingOptions: options,
		ConfigProviderSettings: otelcol.ConfigProviderSettings{
			ResolverSettings: confmap.ResolverSettings{
				URIs: []string{uri},
				ProviderFactories: []confmap.ProviderFactory{
					envprovider.NewFactory(),
					fileprovider.NewFactory(),
				},
				ConverterFactories: extraFactories.GetConverters(),
			},
		},
	}, nil
}

func registerInstrumentation(lc compdef.Lifecycle) error {
	exp, err := otlpmetricgrpc.New(context.Background(), otlpmetricgrpc.WithInsecure())
	if err != nil {
		return err
	}

	// Add go.schedule.duration
	rp := runtime.NewProducer()

	reader := metric.NewPeriodicReader(exp, metric.WithProducer(rp))
	mp := metric.NewMeterProvider(metric.WithReader(reader))

	lc.Append(compdef.Hook{
		OnStart: func(_ context.Context) error {
			return runtime.Start(runtime.WithMeterProvider(mp))
		},
		OnStop: func(ctx context.Context) error {
			return mp.Shutdown(ctx)
		},
	})
	return nil
}
