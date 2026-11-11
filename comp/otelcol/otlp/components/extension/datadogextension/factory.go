// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !aix && !freebsd && !solaris

// Package datadogextension adds optional agentless RC forwarding to the contrib extension.
package datadogextension

import (
	"context"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/extension/datadogextension/internal/remoteconfig"
	upstream "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogextension"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
)

// NewFactory preserves the upstream extension when remote_config is disabled.
func NewFactory(base extension.Factory) extension.Factory {
	return extension.NewFactory(base.Type(), func() component.Config {
		return &Config{Datadog: *base.CreateDefaultConfig().(*upstream.Config), RemoteConfig: remoteconfig.DefaultConfig()}
	}, func(ctx context.Context, set extension.Settings, config component.Config) (extension.Extension, error) {
		c := config.(*Config)
		if err := c.Validate(); err != nil {
			return nil, err
		}
		metadata, err := base.Create(ctx, set, &c.Datadog)
		if err != nil {
			return nil, err
		}
		if c.RemoteConfig == nil || !c.RemoteConfig.Enabled {
			return metadata, nil
		}
		rc, err := remoteconfig.New(set, c.RemoteConfig, string(c.Datadog.API.Key), c.Datadog.API.Site, c.Datadog.ClientConfig)
		if err != nil {
			return nil, err
		}
		return &datadogExtension{metadata: metadata, rc: rc}, nil
	}, base.Stability())
}
