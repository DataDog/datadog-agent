// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !aix && !freebsd && !solaris

// Package datadogextension adds optional RC and DI delivery to the contrib Datadog extension.
package datadogextension

import (
	"context"
	"net/http"

	upstream "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogextension"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/agentcomponents"
	datadogconfig "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"

	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/extension/datadogextension/internal/remoteconfig"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

type wrappedFactory struct{ extension.Factory }

type factory struct {
	base  extension.Factory
	newRC func(extension.Settings, *remoteconfig.Config, model.Reader) (extension.Extension, error)
}

// WrapFactory replaces an existing factory without adding another component identity.
func WrapFactory(factories map[component.Type]extension.Factory) {
	id := component.MustNewType("datadog")
	if base, ok := factories[id]; ok {
		if _, wrapped := base.(*wrappedFactory); wrapped {
			return
		}
		factories[id] = NewFactory(base)
	}
}

// NewFactory preserves upstream behavior unless RC or DI delivery is enabled.
func NewFactory(base extension.Factory) extension.Factory {
	f := &factory{base: base, newRC: remoteconfig.New}
	return &wrappedFactory{Factory: extension.NewFactory(base.Type(), f.defaultConfig, f.create, base.Stability())}
}

func (f *factory) defaultConfig() component.Config {
	return &Config{Datadog: *f.base.CreateDefaultConfig().(*upstream.Config), RemoteConfig: remoteconfig.DefaultConfig()}
}

func (f *factory) create(ctx context.Context, set extension.Settings, config component.Config) (extension.Extension, error) {
	c := config.(*Config)
	if err := c.Validate(); err != nil {
		return nil, err
	}
	metadata, err := f.base.Create(ctx, set, &c.Datadog)
	if err != nil {
		return nil, err
	}
	if (c.RemoteConfig == nil || !c.RemoteConfig.Enabled) && !c.DynamicInstrumentation.Enabled {
		return metadata, nil
	}
	// Upstream keeps its Agent config private; both adapters use the same resolved credentials.
	cfg := &datadogconfig.Config{API: c.Datadog.API, ClientConfig: c.Datadog.ClientConfig}
	reader := agentcomponents.NewConfigComponent(agentcomponents.WithAPIConfig(cfg), agentcomponents.WithProxy(cfg), agentcomponents.WithTLSSetting(cfg))
	var rc extension.Extension
	if c.RemoteConfig != nil && c.RemoteConfig.Enabled {
		rc, err = f.newRC(set, c.rcConfig(), reader)
		if err != nil {
			return nil, err
		}
	}
	e := &datadogExtension{metadata: metadata, rc: rc}
	if c.DynamicInstrumentation.Enabled {
		if err := validateDIConfig(c); err != nil {
			return nil, err
		}
		e.di = &diSender{key: string(c.Datadog.API.Key), endpoint: "https://debugger-intake." + c.Datadog.API.Site + "/api/v2/debugger"}
		e.startDI = func(ctx context.Context, host component.Host) error {
			client, err := c.Datadog.ClientConfig.ToClient(ctx, host.GetExtensions(), set.TelemetrySettings)
			if err != nil {
				return err
			}
			client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			e.di.client = client
			return nil
		}
	}
	return e, nil
}
