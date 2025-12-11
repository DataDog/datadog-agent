// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// Config holds MCP server configuration
type Config struct {
	Enabled        bool
	Address        string
	TLS            *TLSConfig
	MaxRequestSize int
	RequestTimeout time.Duration
	MaxConnections int
	Tools          map[string]ToolConfig
}

// TLSConfig holds TLS configuration
type TLSConfig struct {
	Enabled  bool
	CertFile string
	KeyFile  string
	CAFile   string
}

// ToolConfig holds tool-specific configuration
type ToolConfig struct {
	Enabled bool
	Config  map[string]interface{}
}

// NewConfig creates configuration from agent config
func NewConfig(cfg pkgconfigmodel.Reader) (*Config, error) {
	if !cfg.GetBool("mcp.enabled") {
		return &Config{Enabled: false}, nil
	}

	config := &Config{
		Enabled:        true,
		Address:        cfg.GetString("mcp.server.address"),
		MaxRequestSize: cfg.GetInt("mcp.server.max_request_size"),
		MaxConnections: cfg.GetInt("mcp.server.max_connections"),
		Tools:          make(map[string]ToolConfig),
	}

	// Parse timeout
	if timeout := cfg.GetString("mcp.server.request_timeout"); timeout != "" {
		d, err := time.ParseDuration(timeout)
		if err != nil {
			return nil, err
		}
		config.RequestTimeout = d
	}

	// Parse TLS config if enabled
	if cfg.GetBool("mcp.server.tls.enabled") {
		config.TLS = &TLSConfig{
			Enabled:  true,
			CertFile: cfg.GetString("mcp.server.tls.cert_file"),
			KeyFile:  cfg.GetString("mcp.server.tls.key_file"),
			CAFile:   cfg.GetString("mcp.server.tls.ca_file"),
		}
	}

	// Parse tool configurations
	if cfg.GetBool("mcp.tools.process.enabled") {
		config.Tools["process"] = ToolConfig{
			Enabled: true,
			Config: map[string]interface{}{
				"scrub_args":                 cfg.GetBool("mcp.tools.process.scrub_args"),
				"max_processes_per_request":  cfg.GetInt("mcp.tools.process.max_processes_per_request"),
				"include_container_metadata": cfg.GetBool("mcp.tools.process.include_container_metadata"),
			},
		}
	}

	return config, nil
}
