// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package hpflareextensionimpl defines the OpenTelemetry Extension implementation.
package hpflareextensionimpl

import (
	"errors"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/confmap"
)

var (
	errHTTPEndpointRequired = errors.New("http endpoint required")
)

// Config has the configuration for the extension.
type Config struct {
	HTTPConfig *confighttp.ServerConfig `mapstructure:",squash"`
}

var _ component.Config = (*Config)(nil)

// Validate checks if the extension configuration is valid
func (c *Config) Validate() error {

	if c.HTTPConfig == nil || c.HTTPConfig.Endpoint == "" {
		return errHTTPEndpointRequired
	}

	return nil
}

// Unmarshal a confmap.Conf into the config struct.
func (c *Config) Unmarshal(conf *confmap.Conf) error {
	err := conf.Unmarshal(c)
	if err != nil {
		return err
	}

	return nil
}
