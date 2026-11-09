// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package datadogremoteconfigextension

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
)

// Type identifies the extension in Collector configuration.
var Type = component.MustNewType("datadog_remoteconfig")

// NewFactory creates a Remote Config proxy without Agent dependencies.
func NewFactory() extension.Factory {
	return extension.NewFactory(Type, createDefaultConfig,
		func(_ context.Context, settings extension.Settings, cfg component.Config) (extension.Extension, error) {
			c := cfg.(*Config)
			if err := c.Validate(); err != nil {
				return nil, err
			}
			return &remoteConfigExtension{config: c, settings: settings}, nil
		}, component.StabilityLevelDevelopment)
}
