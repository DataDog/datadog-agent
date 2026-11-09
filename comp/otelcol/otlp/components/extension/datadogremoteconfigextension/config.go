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
	"go.opentelemetry.io/collector/config/configtls"
)

// Config configures the listener and its fixed Remote Config upstream.
type Config struct {
	Endpoint           string                  `mapstructure:"endpoint"`
	TLS                *configtls.ServerConfig `mapstructure:"tls,omitempty"`
	Egress             EgressConfig            `mapstructure:"egress"`
	MaxRequestBodySize int64                   `mapstructure:"max_request_body_size"`
}

// EgressConfig configures the upstream transport without changing RC payloads or credentials.
type EgressConfig struct {
	Endpoint string                 `mapstructure:"endpoint"`
	Timeout  time.Duration          `mapstructure:"timeout"`
	TLS      configtls.ClientConfig `mapstructure:"tls,omitempty"`
	ProxyURL string                 `mapstructure:"proxy_url,omitempty"`
}

func createDefaultConfig() component.Config {
	egress := EgressConfig{Endpoint: "https://config.datadoghq.com", Timeout: 10 * time.Second}
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
	if c.Egress.ProxyURL != "" {
		proxy, err := url.Parse(c.Egress.ProxyURL)
		if err != nil || (proxy.Scheme != "http" && proxy.Scheme != "https") || proxy.Hostname() == "" ||
			(proxy.Path != "" && proxy.Path != "/") || proxy.RawQuery != "" || proxy.ForceQuery || proxy.Fragment != "" {
			return errors.New("egress.proxy_url must be an HTTP or HTTPS proxy origin")
		}
	}
	if c.MaxRequestBodySize <= 0 {
		return errors.New("max_request_body_size must be positive")
	}
	return nil
}
