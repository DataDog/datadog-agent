// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package datadogremoteconfigextension proxies the agentless Remote Config protocol.
package datadogremoteconfigextension

import (
	"errors"
	"net"
	"net/url"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/configtls"
)

// Config configures the listener and its fixed Remote Config upstream.
type Config struct {
	Endpoint           string                  `mapstructure:"endpoint"`
	TLS                *configtls.ServerConfig `mapstructure:"tls,omitempty"`
	Egress             confighttp.ClientConfig `mapstructure:"egress"`
	MaxRequestBodySize int64                   `mapstructure:"max_request_body_size"`
}

func createDefaultConfig() component.Config {
	egress := confighttp.NewDefaultClientConfig()
	egress.Endpoint = "https://config.datadoghq.com"
	egress.Timeout = 10 * time.Second
	return &Config{
		Endpoint:           "127.0.0.1:8234",
		Egress:             egress,
		MaxRequestBodySize: 4 * 1024 * 1024,
	}
}

// Validate checks the listener, upstream and request bounds.
func (c *Config) Validate() error {
	if _, _, err := net.SplitHostPort(c.Endpoint); err != nil {
		return errors.New("endpoint must be a host:port address")
	}
	u, err := url.Parse(c.Egress.Endpoint)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return errors.New("egress.endpoint must be an HTTPS origin without credentials, query or fragment")
	}
	if c.Egress.Timeout <= 0 {
		return errors.New("egress.timeout must be positive")
	}
	if c.Egress.Compression.IsCompressed() {
		return errors.New("egress.compression must be disabled to preserve Remote Config payloads")
	}
	if c.MaxRequestBodySize <= 0 {
		return errors.New("max_request_body_size must be positive")
	}
	return nil
}
