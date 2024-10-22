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
	"go.opentelemetry.io/collector/confmap"
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

// NewFactory creates a factory for Datadog Flare Extension for use with OCB
func NewFactory() extension.Factory {
	return extension.NewFactory(
		metadata.Type,
		createDefaultConfig,
		createExtension,
		metadata.ExtensionStability,
	)
}

// NewFactoryForAgent creates a factory for Datadog Flare Extension for the Agent to use.
func NewFactoryForAgent(factories *otelcol.Factories, configProviderSettings otelcol.ConfigProviderSettings) extension.Factory {
	return &ddExtensionFactory{
		factories:              factories,
		configProviderSettings: configProviderSettings,
	}
}

// createExtension is used for creating extension with OCB
func createExtension(ctx context.Context, set extension.Settings, cfg component.Config) (extension.Extension, error) {
	config := &Config{
		HTTPConfig:             cfg.(*Config).HTTPConfig,
		factories:              &otelcol.Factories{},
		configProviderSettings: otelcol.ConfigProviderSettings{ResolverSettings: confmap.ResolverSettings{URIs: []string{"test"}, ProviderFactories: []confmap.ProviderFactory{}}},
	}
	return NewExtension(ctx, config, set.TelemetrySettings, set.BuildInfo)
}

// CreateExtension exports extension creation for use within Datadog Agent
func (f *ddExtensionFactory) CreateExtension(ctx context.Context, set extension.Settings, cfg component.Config) (extension.Extension, error) {
	config := &Config{
		factories:              f.factories,
		configProviderSettings: f.configProviderSettings,
	}
	config.HTTPConfig = cfg.(*Config).HTTPConfig
	return NewExtensionForAgent(ctx, config, set.TelemetrySettings, set.BuildInfo)
}

// createDefaultConfig is used for creating default configuration with OCB
func createDefaultConfig() component.Config {
	return &Config{
		HTTPConfig: &confighttp.ServerConfig{
			Endpoint: fmt.Sprintf("localhost:%d", defaultHTTPPort),
		},
	}
}

// CreateDefaultConfig exports default configuration for use within Datadog Agent
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

func (f *ddExtensionFactory) ExtensionStability() component.StabilityLevel {
	return metadata.ExtensionStability
}
