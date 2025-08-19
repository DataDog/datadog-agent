// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddprofilingextensionimpl defines the OpenTelemetry Extension implementation.
package ddprofilingextensionimpl

import (
	"context"
	"errors"

	corelog "github.com/DataDog/datadog-agent/comp/core/log/def"
	traceagent "github.com/DataDog/datadog-agent/comp/trace/agent/def"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes/source"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/hostmetadata"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
)

// Type exports the internal metadata type for easy reference
var Type = component.MustNewType("ddprofiling")

type ddExtensionFactory struct {
	extension.Factory
	traceAgent traceagent.Component
	log        corelog.Component
}

// NewFactory creates a factory for Datadog Profiling Extension for use with OCB and OSS Collector
func NewFactory() extension.Factory {
	return &ddExtensionFactory{}
}

// NewFactoryForAgent creates a factory for Datadog Profiling Extension for use with Agent
func NewFactoryForAgent(traceAgent traceagent.Component, log corelog.Component) extension.Factory {
	return &ddExtensionFactory{
		traceAgent: traceAgent,
		log:        log,
	}
}

// Create creates a new instance of the Datadog Profiling Extension
func (f *ddExtensionFactory) Create(_ context.Context, set extension.Settings, cfg component.Config) (extension.Extension, error) {
	config, ok := cfg.(*Config)
	if !ok {
		return nil, errors.New("invalid ddprofiling extension config")
	}
	var sourceProvider source.Provider
	if f.traceAgent == nil {
		var err error
		sourceProvider, err = hostmetadata.GetSourceProvider(set.TelemetrySettings, "", 0)
		if err != nil {
			return nil, err
		}
	}

	return NewExtension(config, set.BuildInfo, f.traceAgent, f.log, sourceProvider)
}

// Stability returns the stability level of the component
func (f *ddExtensionFactory) Stability() component.StabilityLevel {
	return component.StabilityLevelDevelopment
}

func (f *ddExtensionFactory) Type() component.Type {
	return Type
}

func (f *ddExtensionFactory) CreateDefaultConfig() component.Config {
	return &Config{}
}
