// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package extensionimpl defines the OpenTelemetry Extension implementation.
package extensionimpl

import (
	"context"
	"fmt"

	configstore "github.com/DataDog/datadog-agent/comp/otelcol/configstore/def"
	"github.com/DataDog/datadog-agent/comp/otelcol/extension/impl/internal/metadata"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/extension"
)

const (
	defaultHTTPPort = 7777
)

type ddExtensionFactory struct {
	extension.Factory

	configstore configstore.Component
}

// NewFactory creates a factory for HealthCheck extension.
func NewFactory(configstore configstore.Component) extension.Factory {
	return &ddExtensionFactory{
		configstore: configstore,
	}
}

func (f *ddExtensionFactory) CreateExtension(ctx context.Context, set extension.Settings, cfg component.Config) (extension.Extension, error) {

	config := &Config{
		ConfigStore: f.configstore,
	}
	config.HTTPConfig = cfg.(*Config).HTTPConfig
	return NewExtension(ctx, config, set.TelemetrySettings, set.BuildInfo)
}

func (f *ddExtensionFactory) CreateDefaultConfig() component.Config {
	return &Config{
		HTTPConfig: &confighttp.ServerConfig{
			Endpoint: fmt.Sprintf("localhost:%d", defaultHTTPPort),
		},
		ConfigStore: f.configstore,
	}
}

func (f *ddExtensionFactory) Type() component.Type {
	return metadata.Type
}

func (f *ddExtensionFactory) ExtensionStability() component.StabilityLevel {
	return metadata.ExtensionStability
}
