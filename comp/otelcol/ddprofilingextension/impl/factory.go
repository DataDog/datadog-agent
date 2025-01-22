// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddprofilingextensionimpl defines the OpenTelemetry Extension implementation.
package ddprofilingextensionimpl

import (
	"context"

	traceagent "github.com/DataDog/datadog-agent/comp/trace/agent/def"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
)

// Type exports the internal metadata type for easy reference
var Type = component.MustNewType("ddprofiling")

// isOCB returns true if extension was built with OCB
func (f *ddExtensionFactory) isOCB() bool {
	return f.traceAgent == nil
}

type ddExtensionFactory struct {
	extension.Factory
	traceAgent traceagent.Component
}

func NewFactory() extension.Factory {
	return &ddExtensionFactory{}
}

func NewFactoryForAgent(traceAgent traceagent.Component) extension.Factory {
	return &ddExtensionFactory{
		traceAgent: traceAgent,
	}
}

func (f *ddExtensionFactory) Create(ctx context.Context, set extension.Settings, cfg component.Config) (extension.Extension, error) {
	return NewExtension(cfg.(*Config), set.BuildInfo, f.traceAgent)
}

func (f *ddExtensionFactory) Stability() component.StabilityLevel {
	return component.StabilityLevelDevelopment
}

func (f *ddExtensionFactory) Type() component.Type {
	return Type
}

func (f *ddExtensionFactory) CreateDefaultConfig() component.Config {
	return &Config{}
}
