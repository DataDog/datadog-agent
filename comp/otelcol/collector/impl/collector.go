// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

// Package collectorimpl provides the implementation of the collector component for OTel Agent
package collectorimpl

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/converter/expandconverter"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpsprovider"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/otelcol"

	"github.com/open-telemetry/opentelemetry-collector-contrib/connector/datadogconnector"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/DataDog/datadog-agent/comp/core/config"
	corelog "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	collectorcontrib "github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/def"
	collector "github.com/DataDog/datadog-agent/comp/otelcol/collector/def"
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
)

type collectorImpl struct {
	log corelog.Component
	set otelcol.CollectorSettings
	col *otelcol.Collector
}

// Requires declares the input types to the constructor
type Requires struct {
	// Lc specifies the compdef lifecycle settings, used for appending startup
	// and shutdown hooks.
	Lc compdef.Lifecycle

	CollectorContrib collectorcontrib.Component
	URIs             []string

	// Below are dependencies required by Datadog exporter and other Agent functionalities
	Log                 corelog.Component
	Converter           confmap.Converter
	Config              config.Component
	Serializer          serializer.MetricSerializer
	TraceAgent          traceagent.Component
	LogsAgent           optional.Option[logsagentpipeline.Component]
	SourceProvider      serializerexporter.SourceProviderFunc
	Tagger              tagger.Component
	StatsdClientWrapper *metricsclient.StatsdClientWrapper
}

// RequiresNoAgent declares the input types to the constructor with no dependencies on Agent components
type RequiresNoAgent struct {
	// Lc specifies the compdef lifecycle settings, used for appending startup
	// and shutdown hooks.
	Lc compdef.Lifecycle

	CollectorContrib collectorcontrib.Component
	URIs             []string
	Config           config.Component
	Converter        confmap.Converter
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

func newConfigProviderSettings(uris []string, converter confmap.Converter, enhanced bool) otelcol.ConfigProviderSettings {
	converterFactories := []confmap.ConverterFactory{
		expandconverter.NewFactory(),
	}

	if enhanced {
		converterFactories = append(converterFactories, &converterFactory{converter: converter})
	}

	return otelcol.ConfigProviderSettings{
		ResolverSettings: confmap.ResolverSettings{
			URIs: uris,
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

func generateID(group, resource, namespace, name string) string {
	return string(util.GenerateKubeMetadataEntityID(group, resource, namespace, name))
}

func addFactories(reqs Requires, factories otelcol.Factories) {
	if v, ok := reqs.LogsAgent.Get(); ok {
		factories.Exporters[datadogexporter.Type] = datadogexporter.NewFactory(reqs.TraceAgent, reqs.Serializer, v, reqs.SourceProvider, reqs.StatsdClientWrapper)
	} else {
		factories.Exporters[datadogexporter.Type] = datadogexporter.NewFactory(reqs.TraceAgent, reqs.Serializer, nil, reqs.SourceProvider, reqs.StatsdClientWrapper)
	}
	factories.Processors[infraattributesprocessor.Type] = infraattributesprocessor.NewFactory(reqs.Tagger, generateID)
	factories.Connectors[component.MustNewType("datadog")] = datadogconnector.NewFactory()
	factories.Extensions[ddextension.Type] = ddextension.NewFactory(&factories, newConfigProviderSettings(reqs.URIs, reqs.Converter, false))
}

var buildInfo = component.BuildInfo{
	Version:     "v0.104.0",
	Command:     filepath.Base(os.Args[0]),
	Description: "Datadog Agent OpenTelemetry Collector",
}

// NewComponent returns a new instance of the collector component with full Agent functionalities.
func NewComponent(reqs Requires) (Provides, error) {
	factories, err := reqs.CollectorContrib.OTelComponentFactories()
	if err != nil {
		return Provides{}, err
	}
	addFactories(reqs, factories)

	converterEnabled := reqs.Config.GetBool("otelcollector.converter.enabled")
	// Replace default core to use Agent logger
	options := []zap.Option{
		zap.WrapCore(func(zapcore.Core) zapcore.Core {
			return zapAgent.NewZapCore()
		}),
	}
	set := otelcol.CollectorSettings{
		BuildInfo:      buildInfo,
		LoggingOptions: options,
		Factories: func() (otelcol.Factories, error) {
			return factories, nil
		},
		ConfigProviderSettings: newConfigProviderSettings(reqs.URIs, reqs.Converter, converterEnabled),
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

// NewComponentNoAgent returns a new instance of the collector component with no Agent functionalities.
// It is used when there is no Datadog exporter in the OTel Agent config.
func NewComponentNoAgent(reqs RequiresNoAgent) (Provides, error) {
	factories, err := reqs.CollectorContrib.OTelComponentFactories()
	if err != nil {
		return Provides{}, err
	}
	factories.Connectors[component.MustNewType("datadog")] = datadogconnector.NewFactory()
	factories.Extensions[ddextension.Type] = ddextension.NewFactory(&factories, newConfigProviderSettings(reqs.URIs, reqs.Converter, false))

	converterEnabled := reqs.Config.GetBool("otelcollector.converter.enabled")
	set := otelcol.CollectorSettings{
		BuildInfo: buildInfo,
		Factories: func() (otelcol.Factories, error) {
			return factories, nil
		},
		ConfigProviderSettings: newConfigProviderSettings(reqs.URIs, reqs.Converter, converterEnabled),
	}
	col, err := otelcol.NewCollector(set)
	if err != nil {
		return Provides{}, err
	}
	c := &collectorImpl{
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
			if c.log != nil {
				c.log.Errorf("Error running the collector pipeline: %v", err)
			} else {
				log.Printf("Error running the collector pipeline: %v", err)
			}
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
