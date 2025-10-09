// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package collectorimpl implements the collector component interface
package collectorimpl

import (
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl/converternoagent"
	"github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl/receiver"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/infraattributesprocessor"

	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/exporter/debugexporter"
	"go.opentelemetry.io/collector/exporter/otlphttpexporter"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor"
)

// ExtraFactories is an interface that provides extra factories for the collector.
// It is used to provide extra factories for the collector when the Agent Core is available or not.
type ExtraFactories interface {
	GetProcessors() []processor.Factory
	GetConverters() []confmap.ConverterFactory
}

// extraFactoriesWithAgentCore is a struct that implements the ExtraFactories interface when the Agent Core is available.
type extraFactoriesWithAgentCore struct {
	tagger   tagger.Component
	hostname hostname.Component
}

var _ ExtraFactories = (*extraFactoriesWithAgentCore)(nil)

// NewExtraFactoriesWithAgentCore creates a new ExtraFactories instance when the Agent Core is available.
func NewExtraFactoriesWithAgentCore(tagger tagger.Component, hostname hostname.Component) ExtraFactories {
	return extraFactoriesWithAgentCore{
		tagger:   tagger,
		hostname: hostname,
	}
}

func (e extraFactoriesWithAgentCore) GetProcessors() []processor.Factory {
	return []processor.Factory{
		infraattributesprocessor.NewFactoryForAgent(e.tagger, e.hostname.Get),
	}
}

func (e extraFactoriesWithAgentCore) GetConverters() []confmap.ConverterFactory {
	return nil
}

// extraFactoriesWithoutAgentCore is a struct that implements the ExtraFactories interface when the Agent Core is NOT available.
type extraFactoriesWithoutAgentCore struct{}

var _ ExtraFactories = (*extraFactoriesWithoutAgentCore)(nil)

// NewExtraFactoriesWithoutAgentCore creates a new ExtraFactories instance when the Agent Core is not available.
func NewExtraFactoriesWithoutAgentCore() ExtraFactories {
	return extraFactoriesWithoutAgentCore{}
}

// GetProcessors returns the processors for the collector.
func (e extraFactoriesWithoutAgentCore) GetProcessors() []processor.Factory {
	return nil
}

// GetConverters returns the converters for the collector.
func (e extraFactoriesWithoutAgentCore) GetConverters() []confmap.ConverterFactory {
	return []confmap.ConverterFactory{
		converternoagent.NewFactory(),
	}
}

// createFactories creates a function that returns the factories for the collector.
func createFactories(extraFactories ExtraFactories) func() (otelcol.Factories, error) {
	return func() (otelcol.Factories, error) {
		recvMap, err := otelcol.MakeFactoryMap(receiver.NewFactory())
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

		processors, err := otelcol.MakeFactoryMap(extraFactories.GetProcessors()...)
		if err != nil {
			return otelcol.Factories{}, err
		}

		return otelcol.Factories{
			Receivers:  recvMap,
			Exporters:  expMap,
			Processors: processors,
		}, nil
	}
}
