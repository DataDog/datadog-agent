// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package datadogremoteconfigextension

import (
	"errors"
	"net"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/configopaque"
)

type apiConfig struct {
	Key  configopaque.String `mapstructure:"key"`
	Site string              `mapstructure:"site"`
}

// Config configures an Agent-protocol RC endpoint backed by the Agent RC service.
type Config struct {
	ServerConfig    confighttp.ServerConfig `mapstructure:",squash"`
	API             apiConfig               `mapstructure:"api"`
	CacheDirectory  string                  `mapstructure:"cache_directory"`
	RefreshInterval time.Duration           `mapstructure:"refresh_interval"`
	RequestTimeout  time.Duration           `mapstructure:"request_timeout"`
}

func createDefaultConfig() component.Config {
	server := confighttp.NewDefaultServerConfig()
	server.NetAddr.Endpoint = "127.0.0.1:8234"
	server.ReadHeaderTimeout = 5 * time.Second
	server.ReadTimeout = 10 * time.Second
	server.MaxRequestBodySize = 1 << 20
	return &Config{
		ServerConfig:    server,
		API:             apiConfig{Site: "datadoghq.com"},
		RefreshInterval: time.Minute,
		RequestTimeout:  5 * time.Second,
	}
}

var sitePattern = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)*$`)

// Validate checks settings before opening the listener or RC cache.
func (c *Config) Validate() error {
	if strings.TrimSpace(string(c.API.Key)) == "" {
		return errors.New("api.key must be configured in the Collector")
	}
	if !sitePattern.MatchString(c.API.Site) {
		return errors.New("api.site must be a Datadog site hostname")
	}
	if !filepath.IsAbs(c.CacheDirectory) {
		return errors.New("cache_directory must be an absolute path")
	}
	if c.RefreshInterval < 5*time.Second || c.RequestTimeout <= 0 {
		return errors.New("refresh_interval must be at least 5s and request_timeout must be positive")
	}
	if c.ServerConfig.MaxRequestBodySize <= 0 {
		return errors.New("max_request_body_size must be positive")
	}
	if _, _, err := net.SplitHostPort(c.ServerConfig.NetAddr.Endpoint); err != nil {
		return errors.New("endpoint must be a host:port address")
	}
	return nil
}
