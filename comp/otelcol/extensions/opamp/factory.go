// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package opamp

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
)

// NewFactory returns the DDOT opamp extension factory.  It registers under the
// "opamp" component type so that it overrides the upstream opampextension when
// wired into the collector via addFactories().
func NewFactory() extension.Factory {
	return extension.NewFactory(
		Type,
		createDefaultConfig,
		createExtension,
		component.StabilityLevelBeta,
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		Capabilities: Capabilities{
			ReportsEffectiveConfig:          true,
			ReportsHealth:                   true,
			ReportsAvailableComponents:      true,
			AcceptsRestartCommand:           false,
			AcceptsOpAMPConnectionSettings:  true,
			ReportsHeartbeat:                true,
			ReportsConnectionSettingsStatus: true,
			ReportsOwnMetrics:               true,
		},
		PPIDPollInterval: 5 * time.Second,
	}
}

func createExtension(_ context.Context, set extension.Settings, cfg component.Config) (extension.Extension, error) {
	return newExtension(set, cfg.(*Config))
}
