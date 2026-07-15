// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package collectorimpl implements the collector component interface
package collectorimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl/converters"
	"github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl/extensions/hpflareextension"
	"github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl/processor/ddhostnameprocessor"
	profilesreceiver "github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl/receiver"
	"github.com/DataDog/datadog-agent/comp/host-profiler/version"
	ddprofilingextensionimpl "github.com/DataDog/datadog-agent/comp/otelcol/ddprofilingextension/impl"
	dogtelextensionimpl "github.com/DataDog/datadog-agent/comp/otelcol/dogtelextension/impl"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/infraattributesprocessor"
	traceagent "github.com/DataDog/datadog-agent/comp/trace/agent/def"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	zapAgent "github.com/DataDog/datadog-agent/pkg/util/log/zap"
	healthcheckextension "github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/attributesprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/cumulativetodeltaprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/filterprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/k8sattributesprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourcedetectionprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourceprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/filelogreceiver"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/prometheusreceiver"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/exporter/debugexporter"
	"go.opentelemetry.io/collector/exporter/otlphttpexporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.opentelemetry.io/collector/service/telemetry/otelconftelemetry"
)

// ExtraFactories is an interface that provides extra factories for the collector.
// It is used to provide extra factories for the collector when the Agent Core is available or not.
type ExtraFactories interface {
	GetReceivers() []receiver.Factory
	GetProcessors() []processor.Factory
	GetConverters() []confmap.ConverterFactory
	GetExtensions() []extension.Factory
	GetLoggingOptions() []zap.Option
	GetAgentConfig() config.Component
	GetProfilerName() string
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

// GetLoggingOptions returns the logging options for the collector when the Agent Core is available.
func (e extraFactoriesWithAgentCore) GetLoggingOptions() []zap.Option {
	zapCore := zapAgent.NewZapCoreWithDepth(zapCoreStackDepth)
	return []zap.Option{
		zap.WrapCore(func(zapcore.Core) zapcore.Core {
			return zapCore
		}),
	}
}

// GetReceivers returns the receivers for the collector when the Agent Core is available.
func (e extraFactoriesWithAgentCore) GetReceivers() []receiver.Factory {
	return nil
}

// GetAgentConfig returns the Agent Core configuration when the Agent Core is available
func (e extraFactoriesWithAgentCore) GetAgentConfig() config.Component {
	return e.config
}

// GetExtensions returns the extensions for the collector when the Agent Core is available.
func (e extraFactoriesWithAgentCore) GetExtensions() []extension.Factory {
	return []extension.Factory{
		ddprofilingextensionimpl.NewFactoryForAgent(e.traceAgent, e.log),
		hpflareextension.NewFactoryForAgent(e.ipcComp),
	}
}

// GetProcessors returns the processors for the collector when the Agent Core is available.
func (e extraFactoriesWithAgentCore) GetProcessors() []processor.Factory {
	return []processor.Factory{
		infraattributesprocessor.NewFactoryForAgent(e.tagger, e.hostname.Get),
		resourceprocessor.NewFactory(),
	}
}

// GetConverters returns the converters for the collector when the Agent Core is available.
func (e extraFactoriesWithAgentCore) GetConverters() []confmap.ConverterFactory {
	return []confmap.ConverterFactory{}
}

// GetProfilerName returns the name of the profiler when the Agent Core is available.
func (e extraFactoriesWithAgentCore) GetProfilerName() string {
	return version.BundledProfilerName
}

// extraFactoriesWithoutAgentCore is a struct that implements the ExtraFactories interface when the Agent Core is not available.
type extraFactoriesWithoutAgentCore struct {
	config       config.Component
	log          log.Component
	workloadmeta workloadmeta.Component
	tagger       tagger.Component
	hostname     hostname.Component
	serializer   serializer.MetricSerializer
	telemetry    telemetry.Component
	secrets      secrets.Component
}

var _ ExtraFactories = (*extraFactoriesWithoutAgentCore)(nil)

// NewExtraFactoriesWithoutAgentCore creates a new ExtraFactories instance when the Agent Core is not available.
// The components below back the dogtelextension extension, which gives standalone mode its own
// local workloadmeta+tagger (no core Agent needed) for container/image metadata enrichment via
// the infraattributesprocessor.
func NewExtraFactoriesWithoutAgentCore(
	config config.Component,
	log log.Component,
	workloadmeta workloadmeta.Component,
	tagger tagger.Component,
	hostname hostname.Component,
	serializer serializer.MetricSerializer,
	telemetry telemetry.Component,
	secrets secrets.Component,
) ExtraFactories {
	return extraFactoriesWithoutAgentCore{
		config:       config,
		log:          log,
		workloadmeta: workloadmeta,
		tagger:       tagger,
		hostname:     hostname,
		serializer:   serializer,
		telemetry:    telemetry,
		secrets:      secrets,
	}
}

// GetLoggingOptions returns the logging options for the collector when the Agent Core is not available.
func (e extraFactoriesWithoutAgentCore) GetLoggingOptions() []zap.Option {
	return []zap.Option{}
}

// GetReceivers returns the receivers for the collector when the Agent Core is not available.
func (e extraFactoriesWithoutAgentCore) GetReceivers() []receiver.Factory {
	return []receiver.Factory{
		filelogreceiver.NewFactory(),
	}
}

// GetAgentConfig returns the local config in Standalone mode; there is no Core Agent process,
// but dogtelextension still needs a config.Component to read the otel_standalone gate.
func (e extraFactoriesWithoutAgentCore) GetAgentConfig() config.Component {
	return e.config
}

// GetExtensions returns the extensions for the collector.
func (e extraFactoriesWithoutAgentCore) GetExtensions() []extension.Factory {
	return []extension.Factory{
		ddprofilingextensionimpl.NewFactory(),
		healthcheckextension.NewFactory(),
		// ipc is nil: EnableTaggerServer defaults to false, and Start() only touches it when
		// starting the tagger gRPC server, which standalone host-profiler doesn't need since
		// the tagger is consumed in-process by infraattributesprocessor below.
		dogtelextensionimpl.NewFactoryForAgent(e.config, e.log, e.serializer, e.hostname, e.workloadmeta, e.tagger, nil, e.telemetry, e.secrets),
	}
}

// GetProcessors returns the processors for the collector when the Agent Core is not available.
func (e extraFactoriesWithoutAgentCore) GetProcessors() []processor.Factory {
	return []processor.Factory{
		k8sattributesprocessor.NewFactory(),
		resourcedetectionprocessor.NewFactory(),
		resourceprocessor.NewFactory(),
		ddhostnameprocessor.NewFactory(),
		infraattributesprocessor.NewFactoryForAgent(e.tagger, e.hostname.Get),
	}
}

// GetConverters returns the converters for the collector when the Agent Core is not available.
func (e extraFactoriesWithoutAgentCore) GetConverters() []confmap.ConverterFactory {
	return []confmap.ConverterFactory{
		converters.NewFactoryWithoutAgent(),
	}
}

// GetProfilerName returns the name of the profiler when the Agent Core is not available.
func (e extraFactoriesWithoutAgentCore) GetProfilerName() string {
	return version.StandaloneProfilerName
}

// createFactories creates a function that returns the factories for the collector.
func createFactories(extraFactories ExtraFactories) func() (otelcol.Factories, error) {
	return func() (otelcol.Factories, error) {
		receiverFactories := []receiver.Factory{profilesreceiver.NewFactory(extraFactories.GetProfilerName()), otlpreceiver.NewFactory(), prometheusreceiver.NewFactory()}
		receiverFactories = append(receiverFactories, extraFactories.GetReceivers()...)
		receivers, err := otelcol.MakeFactoryMap(receiverFactories...)
		if err != nil {
			return otelcol.Factories{}, err
		}

		exporters, err := otelcol.MakeFactoryMap(
			debugexporter.NewFactory(),
			otlphttpexporter.NewFactory(),
		)
		if err != nil {
			return otelcol.Factories{}, err
		}

		processorFactories := []processor.Factory{
			attributesprocessor.NewFactory(),
			cumulativetodeltaprocessor.NewFactory(),
			filterprocessor.NewFactory(),
		}
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
			Receivers:  receivers,
			Exporters:  exporters,
			Processors: processors,
			Extensions: extensions,
			Telemetry:  otelconftelemetry.NewFactory(),
		}, nil
	}
}
