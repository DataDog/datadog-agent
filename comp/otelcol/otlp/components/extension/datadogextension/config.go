// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !aix && !freebsd && !solaris

package datadogextension

import (
	"errors"

	upstream "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogextension"
	"go.opentelemetry.io/collector/confmap"

	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/extension/datadogextension/internal/remoteconfig"
)

// Config keeps the upstream configuration and adds opt-in RC and DI capabilities.
type Config struct {
	Datadog                upstream.Config      `mapstructure:",squash"`
	RemoteConfig           *remoteconfig.Config `mapstructure:"remote_config"`
	DynamicInstrumentation DIConfig             `mapstructure:"dynamic_instrumentation"`
}

// DIConfig opts into routing named OTLP events to debugger intake.
type DIConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

// Unmarshal rejects an independent RC credential source before decoding settings.
func (c *Config) Unmarshal(conf *confmap.Conf) error {
	if conf.IsSet("remote_config::api") {
		return errors.New("remote_config uses the top-level api.key and api.site; remote_config.api is not supported")
	}
	return conf.Unmarshal(c)
}

func (c *Config) rcConfig() *remoteconfig.Config {
	cfg := *c.RemoteConfig
	cfg.API = remoteconfig.APIConfig{Key: c.Datadog.API.Key, Site: c.Datadog.API.Site}
	cfg.Backend = c.Datadog.ClientConfig
	return &cfg
}

func (c *Config) Validate() error {
	if err := c.Datadog.Validate(); err != nil {
		return err
	}
	if c.DynamicInstrumentation.Enabled {
		if err := validateDIConfig(c); err != nil {
			return err
		}
	}
	if c.RemoteConfig == nil || !c.RemoteConfig.Enabled {
		return nil
	}
	if c.Datadog.ClientConfig.TLS.Insecure || c.Datadog.ClientConfig.TLS.InsecureSkipVerify {
		return errors.New("remote_config requires verified backend TLS")
	}
	if c.Datadog.ClientConfig.Auth.HasValue() || len(c.Datadog.ClientConfig.Middlewares) > 0 || len(c.Datadog.ClientConfig.Headers) > 0 || c.Datadog.ClientConfig.Compression.IsCompressed() {
		return errors.New("remote_config uses api.key for backend authentication; custom auth, middleware, headers and compression are not supported")
	}
	return remoteconfig.ValidateConfig(c.rcConfig())
}
