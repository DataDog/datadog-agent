// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestNewConfigDisabled(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetTestOnlyDynamicSchema(true)
	cfg.SetWithoutSource("mcp.enabled", false)

	config, err := NewConfig(cfg)
	require.NoError(t, err)
	assert.False(t, config.Enabled)
}

func TestNewConfigBasic(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetTestOnlyDynamicSchema(true)
	cfg.SetWithoutSource("mcp.enabled", true)
	cfg.SetWithoutSource("mcp.server.address", "unix:///tmp/test.sock")
	cfg.SetWithoutSource("mcp.server.max_request_size", 1024)
	cfg.SetWithoutSource("mcp.server.max_connections", 50)

	config, err := NewConfig(cfg)
	require.NoError(t, err)
	assert.True(t, config.Enabled)
	assert.Equal(t, "unix:///tmp/test.sock", config.Address)
	assert.Equal(t, 1024, config.MaxRequestSize)
	assert.Equal(t, 50, config.MaxConnections)
}

func TestNewConfigWithTimeout(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetTestOnlyDynamicSchema(true)
	cfg.SetWithoutSource("mcp.enabled", true)
	cfg.SetWithoutSource("mcp.server.request_timeout", "30s")

	config, err := NewConfig(cfg)
	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, config.RequestTimeout)
}

func TestNewConfigInvalidTimeout(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetTestOnlyDynamicSchema(true)
	cfg.SetWithoutSource("mcp.enabled", true)
	cfg.SetWithoutSource("mcp.server.request_timeout", "invalid")

	_, err := NewConfig(cfg)
	assert.Error(t, err)
}

func TestNewConfigWithTLS(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetTestOnlyDynamicSchema(true)
	cfg.SetWithoutSource("mcp.enabled", true)
	cfg.SetWithoutSource("mcp.server.tls.enabled", true)
	cfg.SetWithoutSource("mcp.server.tls.cert_file", "/path/to/cert.pem")
	cfg.SetWithoutSource("mcp.server.tls.key_file", "/path/to/key.pem")
	cfg.SetWithoutSource("mcp.server.tls.ca_file", "/path/to/ca.pem")

	config, err := NewConfig(cfg)
	require.NoError(t, err)
	require.NotNil(t, config.TLS)
	assert.True(t, config.TLS.Enabled)
	assert.Equal(t, "/path/to/cert.pem", config.TLS.CertFile)
	assert.Equal(t, "/path/to/key.pem", config.TLS.KeyFile)
	assert.Equal(t, "/path/to/ca.pem", config.TLS.CAFile)
}

func TestNewConfigWithProcessTool(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetTestOnlyDynamicSchema(true)
	cfg.SetWithoutSource("mcp.enabled", true)
	cfg.SetWithoutSource("mcp.tools.process.enabled", true)
	cfg.SetWithoutSource("mcp.tools.process.scrub_args", true)
	cfg.SetWithoutSource("mcp.tools.process.max_processes_per_request", 500)
	cfg.SetWithoutSource("mcp.tools.process.include_container_metadata", true)

	config, err := NewConfig(cfg)
	require.NoError(t, err)
	require.Contains(t, config.Tools, "process")

	processTool := config.Tools["process"]
	assert.True(t, processTool.Enabled)
	assert.Equal(t, true, processTool.Config["scrub_args"])
	assert.Equal(t, 500, processTool.Config["max_processes_per_request"])
	assert.Equal(t, true, processTool.Config["include_container_metadata"])
}

func TestNewConfigWithoutProcessTool(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetTestOnlyDynamicSchema(true)
	cfg.SetWithoutSource("mcp.enabled", true)
	cfg.SetWithoutSource("mcp.tools.process.enabled", false)

	config, err := NewConfig(cfg)
	require.NoError(t, err)
	assert.NotContains(t, config.Tools, "process")
}
