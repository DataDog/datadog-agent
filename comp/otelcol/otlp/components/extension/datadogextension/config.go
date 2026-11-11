// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !aix && !freebsd && !solaris

package datadogextension

import (
	"errors"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/extension/datadogextension/internal/remoteconfig"
	upstream "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogextension"
	"go.opentelemetry.io/collector/confmap"
	"strings"
)

// Config adds optional agentless RC forwarding to the upstream Datadog extension.
type Config struct {
	Datadog      upstream.Config      `mapstructure:",squash"`
	RemoteConfig *remoteconfig.Config `mapstructure:"remote_config"`
}

// Unmarshal keeps the existing extension API settings as the only credential source.
func (c *Config) Unmarshal(conf *confmap.Conf) error {
	if conf.IsSet("remote_config::api") {
		return errors.New("remote_config uses top-level api.key and api.site")
	}
	return conf.Unmarshal(c)
}

func (c *Config) Validate() error {
	if err := c.Datadog.Validate(); err != nil {
		return err
	}
	if c.RemoteConfig == nil || !c.RemoteConfig.Enabled {
		return nil
	}
	for key := range c.Datadog.ClientConfig.Headers.Iter {
		if strings.EqualFold(key, "DD-API-KEY") {
			return errors.New("RC credentials must use api.key, not HTTP headers")
		}
	}
	return remoteconfig.ValidateConfig(c.RemoteConfig, c.Datadog.API.Site)
}
