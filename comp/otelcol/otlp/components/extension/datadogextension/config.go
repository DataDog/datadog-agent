// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !aix && !freebsd && !solaris

package datadogextension

import (
	"errors"

	upstream "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogextension"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/confmap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/remoteconfig"
)

// Config keeps the upstream configuration and adds an opt-in RC capability.
type Config struct {
	SDKEndpoint              string                                      `mapstructure:"sdk_endpoint"`
	SDKHTTP                  *confighttp.ServerConfig                    `mapstructure:"sdk_http"`
	InstrumentationTelemetry remoteconfig.InstrumentationTelemetryConfig `mapstructure:"instrumentation_telemetry"`
	Datadog                  upstream.Config                             `mapstructure:",squash"`
	RemoteConfig             *remoteconfig.Config                        `mapstructure:"remote_config"`
}

// Unmarshal rejects an independent RC credential source before decoding settings.
func (c *Config) Unmarshal(conf *confmap.Conf) error {
	if conf.IsSet("remote_config::api") {
		return errors.New("remote_config uses the top-level api.key and api.site; remote_config.api is not supported")
	}
	if c.SDKHTTP == nil && conf.IsSet("sdk_http") {
		server := remoteconfig.DefaultConfig().ServerConfig
		c.SDKHTTP = &server
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
	return remoteconfig.ValidateSDKConfig(c.sdkConfig())
}

func (c *Config) sdkConfig() *remoteconfig.SDKConfig {
	return &remoteconfig.SDKConfig{Endpoint: c.SDKEndpoint, HTTP: c.SDKHTTP, RemoteConfig: c.RemoteConfig, InstrumentationTelemetry: c.InstrumentationTelemetry, API: remoteconfig.APIConfig{Key: c.Datadog.API.Key, Site: c.Datadog.API.Site}, Backend: c.Datadog.ClientConfig}
}
