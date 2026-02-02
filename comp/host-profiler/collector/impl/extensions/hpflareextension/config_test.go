// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package hpflareextension defines the OpenTelemetry Extension implementation.
package hpflareextension

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/confmap"
)

func getTestConfig() *Config {
	return &Config{
		HTTPConfig: &confighttp.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  "localhost:0",
				Transport: confignet.TransportTypeTCP,
			},
		},
	}
}

func TestValidate(t *testing.T) {
	cfg := getTestConfig()

	err := cfg.Validate()
	assert.NoError(t, err)

	cfg.HTTPConfig.NetAddr.Endpoint = ""
	err = cfg.Validate()
	assert.ErrorIs(t, err, errHTTPEndpointRequired)

	cfg.HTTPConfig = nil
	err = cfg.Validate()
	assert.ErrorIs(t, err, errHTTPEndpointRequired)
}

func TestUnmarshal(t *testing.T) {
	cfg := getTestConfig()

	endpoint := "localhost:1234"

	m := map[string]any{
		"endpoint": endpoint,
	}

	myConfMap := confmap.NewFromStringMap(m)

	err := myConfMap.Unmarshal(cfg)
	assert.NoError(t, err)

	err = cfg.Validate()
	assert.NoError(t, err)

	assert.Equal(t, endpoint, cfg.HTTPConfig.NetAddr.Endpoint)
}
