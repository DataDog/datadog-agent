// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package agentimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/logs-library/sender/foldspace"
)

func TestBuildEndpoints(t *testing.T) {
	config := config.NewMock(t)

	endpoints, err := buildEndpoints(config)
	assert.Nil(t, err)
	assert.Equal(t, "agent-intake.logs.datadoghq.com.", endpoints.Main.Host)
}

func TestValidateFoldspaceTCP(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetInTest("logs_config.foldspace.enabled", true)
	cfg.SetInTest("logs_config.force_use_tcp", true)
	err := validateFoldspace(cfg)
	assert.Error(t, err)
}

func TestValidateFoldspaceSOCKS5(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetInTest("logs_config.foldspace.enabled", true)
	cfg.SetInTest("logs_config.socks5_proxy_address", "127.0.0.1:1080")
	err := validateFoldspace(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "socks5")
}

func TestValidateFoldspaceMissingTag(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetInTest("logs_config.foldspace.enabled", true)
	err := validateFoldspace(cfg)
	if foldspace.BuiltWithFoldspace {
		assert.NoError(t, err)
		return
	}
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "foldspace tag")
}

func TestFoldspaceAdditionalEndpointsStayHTTP(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetInTest("logs_config.foldspace.enabled", true)
	cfg.SetInTest("logs_config.additional_endpoints", []map[string]interface{}{
		{"api_key": "k", "host": "extra.example", "port": 443},
	})
	endpoints, err := buildEndpoints(cfg)
	assert.NoError(t, err)
	assert.True(t, endpoints.UseHTTP)
}
