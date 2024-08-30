// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddflareextensionimpl defines the OpenTelemetry Extension implementation.
package ddflareextensionimpl

import (
	"errors"
	"fmt"

	configstore "github.com/DataDog/datadog-agent/comp/otelcol/configstore/def"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/confmap"
)

type extractDebugEndpoint func(conf *confmap.Conf) (string, error)

var (
	errHTTPEndpointRequired  = errors.New("http endpoint required")
	supportedDebugExtensions = map[string]extractDebugEndpoint{
		"health_check": healthExtractEndpoint,
		"zpages":       zPagesExtractEndpoint,
		"pprof":        pprofExtractEndpoint,
	}
)

// Config has the configuration for the extension enabling the health check
// extension, used to report the health status of the service.
type Config struct {
	HTTPConfig *confighttp.ServerConfig `mapstructure:",squash"`

	ConfigStore configstore.Component
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

// todo: uncomment once zpages data is re-added to flare
func zPagesExtractEndpoint(c *confmap.Conf) (string, error) {
	endpoint, err := regularStringEndpointExtractor(c)
	return endpoint, err
}

func pprofExtractEndpoint(c *confmap.Conf) (string, error) {
	endpoint, err := regularStringEndpointExtractor(c)
	return endpoint, err
}

func healthExtractEndpoint(c *confmap.Conf) (string, error) {
	endpoint, err := regularStringEndpointExtractor(c)
	return endpoint, err
}

func regularStringEndpointExtractor(c *confmap.Conf) (string, error) {
	if c == nil {
		return "", fmt.Errorf("nil confmap - skipping")
	}

	element := c.Get("endpoint")
	if element == nil {
		return "", fmt.Errorf("Expected endpoint conf element, but none found")
	}

	endpoint, ok := element.(string)
	if !ok {
		return "", fmt.Errorf("endpoint conf element was unexpectedly not a string")
	}
	return endpoint, nil
}
