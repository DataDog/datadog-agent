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

	"github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/impl/internal/metadata"
)

const (
	defaultHTTPPort = 7777
)

type ddExtensionFactory struct {
	extension.Factory

	factories              *otelcol.Factories
	configProviderSettings otelcol.ConfigProviderSettings
}

func (f *ddExtensionFactory) isOCB() bool {
	return f.factories == nil
}

// NewFactory creates a factory for Datadog Flare Extension for use with OCB and OSS Collector
func NewFactory() extension.Factory {
	return &ddExtensionFactory{}
}

// NewFactoryForAgent creates a factory for Datadog Flare Extension for use with Agent
func NewFactoryForAgent(factories *otelcol.Factories, configProviderSettings otelcol.ConfigProviderSettings) extension.Factory {
	return &ddExtensionFactory{
		factories:              factories,
		configProviderSettings: configProviderSettings,
	}
}

// CreateExtension creates a new instance of the Datadog Flare Extension, deprecated as of v0.112.0
// TODO: Remove CreateExtension when updating collector dependencies to v0.112.0 or later
func (f *ddExtensionFactory) CreateExtension(ctx context.Context, set extension.Settings, cfg component.Config) (extension.Extension, error) {
	config := &Config{
		factories:              f.factories,
		configProviderSettings: f.configProviderSettings,
	}
	config.HTTPConfig = cfg.(*Config).HTTPConfig
	providedConfigSupported := f.isOCB()
	return NewExtension(ctx, config, set.TelemetrySettings, set.BuildInfo, providedConfigSupported)
}

// Create creates a new instance of the Datadog Flare Extension, as of v0.112.0 or later
func (f *ddExtensionFactory) Create(ctx context.Context, set extension.Settings, cfg component.Config) (extension.Extension, error) {
	config := &Config{
		factories:              f.factories,
		configProviderSettings: f.configProviderSettings,
	}
	config.HTTPConfig = cfg.(*Config).HTTPConfig
	providedConfigSupported := f.isOCB()
	return NewExtension(ctx, config, set.TelemetrySettings, set.BuildInfo, providedConfigSupported)
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
// TODO: remove ExtensionStability when updating collector dependencies to v0.112.0 or later
func (f *ddExtensionFactory) ExtensionStability() component.StabilityLevel {
	return metadata.ExtensionStability
}

// Stability is the new stability level interface for extension as of v0.112.0
func (f *ddExtensionFactory) Stability() component.StabilityLevel {
	return metadata.ExtensionStability
}
