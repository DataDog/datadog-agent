// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package remoteconfig provides stateless agentless RC forwarding for DDOT.
package remoteconfig

import (
	"errors"
	"go.opentelemetry.io/collector/config/confighttp"
	"net"
	"regexp"
	"time"
)

// Config controls the private agentless RC listener.
type Config struct {
	Enabled        bool                    `mapstructure:"enabled"`
	ServerConfig   confighttp.ServerConfig `mapstructure:",squash"`
	RequestTimeout time.Duration           `mapstructure:"request_timeout"`
}

func DefaultConfig() *Config {
	server := confighttp.NewDefaultServerConfig()
	server.NetAddr.Endpoint = "127.0.0.1:8234"
	server.ReadHeaderTimeout = 5 * time.Second
	server.ReadTimeout = 10 * time.Second
	server.MaxRequestBodySize = 1 << 20
	return &Config{ServerConfig: server, RequestTimeout: 10 * time.Second}
}

var sitePattern = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)*$`)

// ValidateConfig checks resolved settings without adding a recursively validated child.
func ValidateConfig(c *Config, site string) error {
	if !sitePattern.MatchString(site) {
		return errors.New("api.site must be a Datadog site hostname")
	}
	if c.RequestTimeout <= 0 {
		return errors.New("remote_config.request_timeout must be positive")
	}
	if c.ServerConfig.MaxRequestBodySize <= 0 {
		return errors.New("remote_config.max_request_body_size must be positive")
	}
	if _, _, err := net.SplitHostPort(c.ServerConfig.NetAddr.Endpoint); err != nil {
		return errors.New("remote_config.endpoint must be host:port")
	}
	return nil
}
