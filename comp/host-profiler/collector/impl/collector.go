// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package collectorimpl implements the collector component interface
package collectorimpl

import (
	"context"

	collector "github.com/DataDog/datadog-agent/comp/host-profiler/collector/def"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/exporter/debugexporter"
	"go.opentelemetry.io/collector/exporter/otlphttpexporter"
	"go.opentelemetry.io/collector/featuregate"
	"go.opentelemetry.io/collector/otelcol"
	ebpfcollector "go.opentelemetry.io/ebpf-profiler/collector"
)

// Params contains the parameters for the collector component.
type Params struct {
	uri string
}

// NewParams creates a new Params instance.
func NewParams(uri string) Params {
	return Params{
		uri: uri,
	}
}

// Requires defines the dependencies for the collector component
type Requires struct {
	Params Params
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

	settings, err := newCollectorSettings(reqs.Params.uri)
	if err != nil {
		return Provides{}, err
	}
	collector, err := otelcol.NewCollector(settings)
	if err != nil {
		return Provides{}, err
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

func newCollectorSettings(uri string) (otelcol.CollectorSettings, error) {
	return otelcol.CollectorSettings{
		Factories: createFactories,
		ConfigProviderSettings: otelcol.ConfigProviderSettings{
			ResolverSettings: confmap.ResolverSettings{
				URIs: []string{uri},
				ProviderFactories: []confmap.ProviderFactory{
					envprovider.NewFactory(),
					fileprovider.NewFactory(),
				},
			},
		},
	}, nil
}

func createFactories() (otelcol.Factories, error) {
	recvMap, err := otelcol.MakeFactoryMap(ebpfcollector.NewFactory())
	if err != nil {
		return otelcol.Factories{}, err
	}

	expMap, err := otelcol.MakeFactoryMap(
		debugexporter.NewFactory(),
		otlphttpexporter.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	return otelcol.Factories{
		Receivers: recvMap,
		Exporters: expMap,
	}, nil
}
