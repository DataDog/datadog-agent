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
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/converter/expandconverter"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpsprovider"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/otelcol"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	collectorcontrib "github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/def"
	collector "github.com/DataDog/datadog-agent/comp/otelcol/collector/def"
	configstore "github.com/DataDog/datadog-agent/comp/otelcol/configstore/def"
	ddextension "github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/impl"
	"github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/datadogexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/infraattributesprocessor"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/datatype"
	traceagent "github.com/DataDog/datadog-agent/comp/trace/agent/def"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	zapAgent "github.com/DataDog/datadog-agent/pkg/util/log/zap"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/open-telemetry/opentelemetry-collector-contrib/connector/datadogconnector"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type collectorImpl struct {
	log log.Component
	set otelcol.CollectorSettings
	col *otelcol.Collector
}

// Requires declares the input types to the constructor
type Requires struct {
	// Lc specifies the compdef lifecycle settings, used for appending startup
	// and shutdown hooks.
	Lc compdef.Lifecycle

	// Log specifies the logging component.
	Log                 log.Component
	Provider            confmap.Converter
	ConfigStore         configstore.Component
	Config              config.Component
	CollectorContrib    collectorcontrib.Component
	Serializer          serializer.MetricSerializer
	TraceAgent          traceagent.Component
	LogsAgent           optional.Option[logsagentpipeline.Component]
	SourceProvider      serializerexporter.SourceProviderFunc
	Tagger              tagger.Component
	StatsdClientWrapper *metricsclient.StatsdClientWrapper
	URIs                []string
}

// Provides declares the output types from the constructor
type Provides struct {
	compdef.Out

	Comp collector.Component
}

type converterFactory struct {
	converter confmap.Converter
}

func (c *converterFactory) Create(_ confmap.ConverterSettings) confmap.Converter {
	return c.converter
}

func newConfigProviderSettings(reqs Requires, enhanced bool) otelcol.ConfigProviderSettings {
	converterFactories := []confmap.ConverterFactory{
		expandconverter.NewFactory(),
	}

	if enhanced {
		converterFactories = append(converterFactories, &converterFactory{converter: reqs.Provider})
	}

	return otelcol.ConfigProviderSettings{
		ResolverSettings: confmap.ResolverSettings{
			URIs: reqs.URIs,
			ProviderFactories: []confmap.ProviderFactory{
				fileprovider.NewFactory(),
				envprovider.NewFactory(),
				yamlprovider.NewFactory(),
				httpprovider.NewFactory(),
				httpsprovider.NewFactory(),
			},
			ConverterFactories: converterFactories,
		},
	}
}
func generateId(group, resource, namespace, name string) string {

	return string(util.GenerateKubeMetadataEntityID(group, resource, namespace, name))
}
func addFactories(reqs Requires, factories otelcol.Factories) {
	if v, ok := reqs.LogsAgent.Get(); ok {
		factories.Exporters[datadogexporter.Type] = datadogexporter.NewFactory(reqs.TraceAgent, reqs.Serializer, v, reqs.SourceProvider, reqs.StatsdClientWrapper)
	} else {
		factories.Exporters[datadogexporter.Type] = datadogexporter.NewFactory(reqs.TraceAgent, reqs.Serializer, nil, reqs.SourceProvider, reqs.StatsdClientWrapper)
	}
	factories.Processors[infraattributesprocessor.Type] = infraattributesprocessor.NewFactory(reqs.Tagger, generateId)
	factories.Extensions[ddextension.Type] = ddextension.NewFactory(reqs.ConfigStore)
	factories.Connectors[component.MustNewType("datadog")] = datadogconnector.NewFactory()
}

// NewComponent returns a new instance of the collector component.
func NewComponent(reqs Requires) (Provides, error) {
	factories, err := reqs.CollectorContrib.OTelComponentFactories()
	if err != nil {
		return Provides{}, err
	}
	addFactories(reqs, factories)

	converterEnabled := reqs.Config.GetBool("otelcollector.converter.enabled")
	err = reqs.ConfigStore.AddConfigs(newConfigProviderSettings(reqs, false), newConfigProviderSettings(reqs, converterEnabled), factories)
	if err != nil {
		return Provides{}, err
	}

	// Replace default core to use Agent logger
	options := []zap.Option{
		zap.WrapCore(func(zapcore.Core) zapcore.Core {
			return zapAgent.NewZapCore()
		}),
	}
	set := otelcol.CollectorSettings{
		BuildInfo: component.BuildInfo{
			Version:     "v0.104.0",
			Command:     "otel-agent",
			Description: "Datadog Agent OpenTelemetry Collector",
		},
		LoggingOptions: options,
		Factories: func() (otelcol.Factories, error) {
			return factories, nil
		},
		ConfigProviderSettings: newConfigProviderSettings(reqs, converterEnabled),
	}
	col, err := otelcol.NewCollector(set)
	if err != nil {
		return Provides{}, err
	}
	c := &collectorImpl{
		log: reqs.Log,
		set: set,
		col: col,
	}

	reqs.Lc.Append(compdef.Hook{
		OnStart: c.start,
		OnStop:  c.stop,
	})
	return Provides{
		Comp: c,
	}, nil
}

func (c *collectorImpl) start(ctx context.Context) error {
	// Dry run the collector pipeline to ensure it is configured correctly
	err := c.col.DryRun(ctx)
	if err != nil {
		return err
	}
	go func() {
		if err := c.col.Run(context.Background()); err != nil {
			c.log.Errorf("Error running the collector pipeline: %v", err)
		}
	}()
	return nil
}

func (c *collectorImpl) stop(context.Context) error {
	c.col.Shutdown()
	return nil
}

func (c *collectorImpl) Status() datatype.CollectorStatus {
	return datatype.CollectorStatus{
		Status: c.col.GetState().String(),
	}
}
