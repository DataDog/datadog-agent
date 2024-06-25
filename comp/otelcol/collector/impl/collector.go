// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

// Package collectorimpl provides the implementation of the collector component for OTel Agent
package collectorimpl

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/converter/expandconverter"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpsprovider"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/otelcol"
	"gopkg.in/yaml.v2"

	corelog "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	collectorcontrib "github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/def"
	collector "github.com/DataDog/datadog-agent/comp/otelcol/collector/def"
	ddextension "github.com/DataDog/datadog-agent/comp/otelcol/extension/impl"
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
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type collectorImpl struct {
	log      corelog.Component
	set      otelcol.CollectorSettings
	col      *otelcol.Collector
	confDump confDump
}

type confDump struct {
	provided *otelcol.Config
	enhanced *otelcol.Config
}

// Requires declares the input types to the constructor
type Requires struct {
	// Lc specifies the compdef lifecycle settings, used for appending startup
	// and shutdown hooks.
	Lc compdef.Lifecycle

	// Log specifies the logging component.
	Log                 corelog.Component
	Provider            confmap.Converter
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

// getConfig returns the *otelcol.Config from the slice of URIs. If enhanced is
// true, it returns the enhanced config, else it returns the provided config.
func getConfig(reqs Requires, enhanced bool) (*otelcol.Config, error) {
	converterFactories := []confmap.ConverterFactory{
		expandconverter.NewFactory(),
	}

	if enhanced {
		converterFactories = []confmap.ConverterFactory{
			expandconverter.NewFactory(),
			&converterFactory{converter: reqs.Provider},
		}
	}

	ocp, err := otelcol.NewConfigProvider(otelcol.ConfigProviderSettings{
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
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create configprovider: %w", err)
	}

	factories, err := reqs.CollectorContrib.OTelComponentFactories()
	if err != nil {
		return nil, err
	}

	if v, ok := reqs.LogsAgent.Get(); ok {
		factories.Exporters[datadogexporter.Type] = datadogexporter.NewFactory(reqs.Serializer, v, reqs.SourceProvider)
	} else {
		factories.Exporters[datadogexporter.Type] = datadogexporter.NewFactory(reqs.Serializer, nil, reqs.SourceProvider)
	}
	factories.Processors[infraattributesprocessor.Type] = infraattributesprocessor.NewFactory(reqs.Tagger)

	conf, err := ocp.Get(context.Background(), factories)
	if err != nil {
		return nil, err
	}

	return conf, nil
}

// NewComponent returns a new instance of the collector component.
func NewComponent(reqs Requires) (Provides, error) {
	// Replace default core to use Agent logger
	options := []zap.Option{
		zap.WrapCore(func(zapcore.Core) zapcore.Core {
			return zapAgent.NewZapCore()
		}),
	}
	set := otelcol.CollectorSettings{
		BuildInfo: component.BuildInfo{
			Version:     "v0.102.0",
			Command:     "otel-agent",
			Description: "Datadog Agent OpenTelemetry Collector Distribution",
		},
		LoggingOptions: options,
		Factories: func() (otelcol.Factories, error) {
			factories, err := reqs.CollectorContrib.OTelComponentFactories()
			if err != nil {
				return otelcol.Factories{}, err
			}
			if v, ok := reqs.LogsAgent.Get(); ok {
				factories.Exporters[datadogexporter.Type] = datadogexporter.NewFactory(reqs.TraceAgent, reqs.Serializer, v, reqs.SourceProvider, reqs.StatsdClientWrapper)
			} else {
				factories.Exporters[datadogexporter.Type] = datadogexporter.NewFactory(reqs.TraceAgent, reqs.Serializer, nil, reqs.SourceProvider, reqs.StatsdClientWrapper)
			}
			factories.Processors[infraattributesprocessor.Type] = infraattributesprocessor.NewFactory(reqs.Tagger)
			factories.Extensions[ddextension.Type] = ddextension.NewFactory(reqs.Provider)
			return factories, nil
		},
		ConfigProviderSettings: otelcol.ConfigProviderSettings{
			ResolverSettings: confmap.ResolverSettings{
				URIs: reqs.URIs,
				ProviderFactories: []confmap.ProviderFactory{
					fileprovider.NewFactory(),
					envprovider.NewFactory(),
					yamlprovider.NewFactory(),
					httpprovider.NewFactory(),
					httpsprovider.NewFactory(),
				},
				ConverterFactories: []confmap.ConverterFactory{
					expandconverter.NewFactory(),
					&converterFactory{converter: reqs.Provider},
				},
			},
		},
	}
	col, err := otelcol.NewCollector(set)
	if err != nil {
		return Provides{}, err
	}
	providedConf, err := getConfig(reqs, false)
	if err != nil {
		return Provides{}, err
	}
	enhancedConf, err := getConfig(reqs, true)
	if err != nil {
		return Provides{}, err
	}
	c := &collectorImpl{
		log: reqs.Log,
		set: set,
		col: col,
		confDump: confDump{
			provided: providedConf,
			enhanced: enhancedConf,
		},
	}

	reqs.Lc.Append(compdef.Hook{
		OnStart: c.start,
		OnStop:  c.stop,
	})
	return Provides{
		Comp: c,
	}, nil
}

// GetProvidedConf returns a string representing the enhanced collector configuration.
func (c *collectorImpl) GetProvidedConf() (*confmap.Conf, error) {
	conf := confmap.New()
	conf.Marshal(c.confDump.provided)
	return conf, nil
}

// GetEnhancedConf returns a string representing the enhanced collector configuration.
func (c *collectorImpl) GetEnhancedConf() (*confmap.Conf, error) {
	conf := confmap.New()
	conf.Marshal(c.confDump.enhanced)
	return conf, nil
}

// GetProvidedConf returns a string representing the enhanced collector configuration.
func (c *collectorImpl) GetProvidedConfAsString() (string, error) {
	confstr, err := confToString(c.confDump.provided)

	return confstr, err
}

// GetEnhancedConf returns a string representing the enhanced collector configuration.
func (c *collectorImpl) GetEnhancedConfAsString() (string, error) {
	confstr, err := confToString(c.confDump.enhanced)

	return confstr, err
}

func confToString(conf *otelcol.Config) (string, error) {
	cfg := confmap.New()
	err := cfg.Marshal(conf)
	if err != nil {
		return "", err
	}
	bytesConf, err := yaml.Marshal(cfg.ToStringMap())
	if err != nil {
		return "", err
	}

	return string(bytesConf), nil
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
