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

	flarebuilder "github.com/DataDog/datadog-agent/comp/core/flare/builder"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	corelog "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	collectorcontrib "github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/def"
	collector "github.com/DataDog/datadog-agent/comp/otelcol/collector/def"
	ddextension "github.com/DataDog/datadog-agent/comp/otelcol/extension/impl"
	"github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/datadogexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/infraattributesprocessor"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/datatype"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

type collectorImpl struct {
	log          corelog.Component
	set          otelcol.CollectorSettings
	col          *otelcol.Collector
	flareEnabled bool
}

// Requires declares the input types to the constructor
type Requires struct {
	// Lc specifies the compdef lifecycle settings, used for appending startup
	// and shutdown hooks.
	Lc compdef.Lifecycle

	// Log specifies the logging component.
	Log              corelog.Component
	Provider         confmap.Converter
	CollectorContrib collectorcontrib.Component
	Serializer       serializer.MetricSerializer
	LogsAgent        optional.Option[logsagentpipeline.Component]
	HostName         hostname.Component
	Tagger           tagger.Component
	URIs             []string
}

// Provides declares the output types from the constructor
type Provides struct {
	compdef.Out

	Comp          collector.Component
	FlareProvider flarebuilder.Provider
}

type converterFactory struct {
	converter confmap.Converter
}

func (c *converterFactory) Create(_ confmap.ConverterSettings) confmap.Converter {
	return c.converter
}

// NewComponent returns a new instance of the collector component.
func NewComponent(reqs Requires) (Provides, error) {
	set := otelcol.CollectorSettings{
		BuildInfo: component.BuildInfo{
			Version:     "0.0.1",
			Command:     "otel-agent",
			Description: "Datadog Agent OpenTelemetry Collector Distribution",
		},
		Factories: func() (otelcol.Factories, error) {
			factories, err := reqs.CollectorContrib.OTelComponentFactories()
			if err != nil {
				return otelcol.Factories{}, err
			}
			if v, ok := reqs.LogsAgent.Get(); ok {
				factories.Exporters[datadogexporter.Type] = datadogexporter.NewFactory(reqs.Serializer, v, reqs.HostName)
			} else {
				factories.Exporters[datadogexporter.Type] = datadogexporter.NewFactory(reqs.Serializer, nil, reqs.HostName)
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
		Comp:          c,
		FlareProvider: flarebuilder.NewProvider(c.fillFlare),
	}, nil
}

func (c *collectorImpl) start(context.Context) error {
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

func (c *collectorImpl) fillFlare(fb flarebuilder.FlareBuilder) error {
	if c.flareEnabled {
		// TODO: placeholder for now, until OTel extension exists to provide data
		fb.AddFile("otel-agent.log", []byte("otel-agent flare")) //nolint:errcheck
	}
	return nil
}

func (c *collectorImpl) Status() datatype.CollectorStatus {
	return datatype.CollectorStatus{
		Status: c.col.GetState().String(),
	}
}
