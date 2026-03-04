// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package collectorimpl implements the collector component interface
package collectorimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl/converters"
	"github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl/extensions/hpflareextension"
	"github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl/receiver"
	ddprofilingextensionimpl "github.com/DataDog/datadog-agent/comp/otelcol/ddprofilingextension/impl"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/infraattributesprocessor"
	traceagent "github.com/DataDog/datadog-agent/comp/trace/agent/def"
	zapAgent "github.com/DataDog/datadog-agent/pkg/util/log/zap"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/attributesprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/cumulativetodeltaprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/k8sattributesprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourcedetectionprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourceprocessor"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/exporter/debugexporter"
	"go.opentelemetry.io/collector/exporter/otlphttpexporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.opentelemetry.io/collector/service/telemetry/otelconftelemetry"
)

// ExtraFactories is an interface that provides extra factories for the collector.
// It is used to provide extra factories for the collector when the Agent Core is available or not.
type ExtraFactories interface {
	GetProcessors() []processor.Factory
	GetConverters() []confmap.ConverterFactory
	GetExtensions() []extension.Factory
	GetLoggingOptions() []zap.Option
}

// extraFactoriesWithAgentCore is a struct that implements the ExtraFactories interface when the Agent Core is available.
type extraFactoriesWithAgentCore struct {
	tagger     tagger.Component
	hostname   hostname.Component
	ipcComp    ipc.Component
	traceAgent traceagent.Component
	log        log.Component
	config     config.Component
}

var _ ExtraFactories = (*extraFactoriesWithAgentCore)(nil)

const (
	// zapCoreStackDepth skips the slog handler and wrapper frames in the logging
	// pipeline to show the actual caller location in log output.
	zapCoreStackDepth = 7
)

// NewExtraFactoriesWithAgentCore creates a new ExtraFactories instance when the Agent Core is available.
func NewExtraFactoriesWithAgentCore(
	tagger tagger.Component,
	hostname hostname.Component, ipcComp ipc.Component,
	traceAgent traceagent.Component,
	log log.Component,
	config config.Component,
) ExtraFactories {
	return extraFactoriesWithAgentCore{
		tagger:     tagger,
		hostname:   hostname,
		ipcComp:    ipcComp,
		traceAgent: traceAgent,
		log:        log,
		config:     config,
	}
}

func (e extraFactoriesWithAgentCore) GetLoggingOptions() []zap.Option {
	zapCore := zapAgent.NewZapCoreWithDepth(zapCoreStackDepth)
	return []zap.Option{
		zap.WrapCore(func(zapcore.Core) zapcore.Core {
			return zapCore
		}),
	}
}

func (e extraFactoriesWithAgentCore) GetExtensions() []extension.Factory {
	return []extension.Factory{
		ddprofilingextensionimpl.NewFactoryForAgent(e.traceAgent, e.log),
		hpflareextension.NewFactoryForAgent(e.ipcComp),
	}
}

func (e extraFactoriesWithAgentCore) GetProcessors() []processor.Factory {
	return []processor.Factory{
		infraattributesprocessor.NewFactoryForAgent(e.tagger, e.hostname.Get),
		resourceprocessor.NewFactory(),
	}
}

func (e extraFactoriesWithAgentCore) GetConverters() []confmap.ConverterFactory {
	return []confmap.ConverterFactory{
		converters.NewFactoryWithAgent(e.config),
	}
}

// extraFactoriesWithoutAgentCore is a struct that implements the ExtraFactories interface when the Agent Core is NOT available.
type extraFactoriesWithoutAgentCore struct{}

var _ ExtraFactories = (*extraFactoriesWithoutAgentCore)(nil)

// NewExtraFactoriesWithoutAgentCore creates a new ExtraFactories instance when the Agent Core is not available.
func NewExtraFactoriesWithoutAgentCore() ExtraFactories {
	return extraFactoriesWithoutAgentCore{}
}

func (e extraFactoriesWithoutAgentCore) GetLoggingOptions() []zap.Option {
	return []zap.Option{}
}

// GetExtensions returns the extensions for the collector.
func (e extraFactoriesWithoutAgentCore) GetExtensions() []extension.Factory {
	return []extension.Factory{}
}

// GetProcessors returns the processors for the collector.
func (e extraFactoriesWithoutAgentCore) GetProcessors() []processor.Factory {
	return []processor.Factory{
		k8sattributesprocessor.NewFactory(),
		resourcedetectionprocessor.NewFactory(),
		resourceprocessor.NewFactory(),
	}
}

// GetConverters returns the converters for the collector.
func (e extraFactoriesWithoutAgentCore) GetConverters() []confmap.ConverterFactory {
	return []confmap.ConverterFactory{
		converters.NewFactoryWithoutAgent(),
	}
}

// createFactories creates a function that returns the factories for the collector.
func createFactories(extraFactories ExtraFactories) func() (otelcol.Factories, error) {
	return func() (otelcol.Factories, error) {
		recvMap, err := otelcol.MakeFactoryMap(receiver.NewFactory(), otlpreceiver.NewFactory())
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

		processorFactories := []processor.Factory{attributesprocessor.NewFactory(), cumulativetodeltaprocessor.NewFactory()}
		processorFactories = append(processorFactories, extraFactories.GetProcessors()...)
		processors, err := otelcol.MakeFactoryMap(processorFactories...)
		if err != nil {
			return otelcol.Factories{}, err
		}

		extensionFactories := extraFactories.GetExtensions()
		extensions, err := otelcol.MakeFactoryMap(extensionFactories...)
		if err != nil {
			return otelcol.Factories{}, err
		}

		return otelcol.Factories{
			Receivers:  recvMap,
			Exporters:  expMap,
			Processors: processors,
			Extensions: extensions,
			Telemetry:  otelconftelemetry.NewFactory(),
		}, nil
	}
}
