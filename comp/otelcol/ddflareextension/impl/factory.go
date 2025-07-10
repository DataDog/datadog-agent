// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddflareextensionimpl defines the OpenTelemetry Extension implementation.
package ddflareextensionimpl

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/otelcol"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/impl/internal/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	defaultHTTPPort = 7777
)

type ddExtensionFactory struct {
	extension.Factory

	factories              *otelcol.Factories
	configProviderSettings otelcol.ConfigProviderSettings
	byoc                   bool
	ipcComp                option.Option[ipc.Component]
}

// isOCB returns true if extension was built with OCB
func (f *ddExtensionFactory) isOCB() bool {
	return f.factories == nil
}

// NewFactory creates a factory for Datadog Flare Extension for use with OCB and OSS Collector
func NewFactory() extension.Factory {
	return &ddExtensionFactory{}
}

// NewFactoryForAgent creates a factory for Datadog Flare Extension for use with Agent
func NewFactoryForAgent(factories *otelcol.Factories, configProviderSettings otelcol.ConfigProviderSettings, ipcComp option.Option[ipc.Component], byoc bool) extension.Factory {
	return &ddExtensionFactory{
		factories:              factories,
		configProviderSettings: configProviderSettings,
		byoc:                   byoc,
		ipcComp:                ipcComp,
	}
}

// CreateExtension is deprecated as of v0.112.0
func (f *ddExtensionFactory) CreateExtension(ctx context.Context, set extension.Settings, cfg component.Config) (extension.Extension, error) {
	config := &Config{
		factories:              f.factories,
		configProviderSettings: f.configProviderSettings,
	}
	config.HTTPConfig = cfg.(*Config).HTTPConfig
	return NewExtension(ctx, config, set.TelemetrySettings, set.BuildInfo, f.ipcComp, !f.isOCB(), f.byoc)
}

// Create creates a new instance of the Datadog Flare Extension, as of v0.112.0 or later
func (f *ddExtensionFactory) Create(ctx context.Context, set extension.Settings, cfg component.Config) (extension.Extension, error) {
	config := &Config{
		factories:              f.factories,
		configProviderSettings: f.configProviderSettings,
	}
	config.HTTPConfig = cfg.(*Config).HTTPConfig
	return NewExtension(ctx, config, set.TelemetrySettings, set.BuildInfo, f.ipcComp, !f.isOCB(), f.byoc)
}

func (f *ddExtensionFactory) CreateDefaultConfig() component.Config {
	return &Config{
		HTTPConfig: &confighttp.ServerConfig{
			Endpoint: fmt.Sprintf("localhost:%d", defaultHTTPPort),
		},
	}
}

func (f *ddExtensionFactory) Type() component.Type {
	return metadata.Type
}

// ExtensionStability is deprecated as of v0.112.0
func (f *ddExtensionFactory) ExtensionStability() component.StabilityLevel {
	return metadata.ExtensionStability
}

// Stability returns the stability level of the component as of v0.112.0 or later
func (f *ddExtensionFactory) Stability() component.StabilityLevel {
	return metadata.ExtensionStability
}
