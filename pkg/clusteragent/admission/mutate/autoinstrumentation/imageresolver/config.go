// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package imageresolver provides configuration and utilities for resolving
// container image references from mutable tags to digests.
package imageresolver

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// RemoteConfigClient defines the interface we need for remote config operations
type RemoteConfigClient interface {
	GetConfigs(product string) map[string]state.RawConfig
	Subscribe(product string, callback func(map[string]state.RawConfig, func(string, state.ApplyStatus)))
}

// Config contains information needed to create an ImageResolver
type Config struct {
	Site           string
	DDRegistries   []string
	RCClient       RemoteConfigClient
	MaxInitRetries int
	InitRetryDelay time.Duration
}

// NewConfig creates a new Config
func NewConfig(cfg config.Component, rcClient RemoteConfigClient) Config {
	return Config{
		Site:           cfg.GetString("site"),
		DDRegistries:   cfg.GetStringMap("admission_controller.auto_instrumentation.default_dd_registries"),
		RCClient:       rcClient,
		MaxInitRetries: 5,
		InitRetryDelay: 1 * time.Second,
	}
}

// NewTestConfig creates a new Config for testing
func NewTestConfig(site string, ddRegistries map[string]any, rcClient RemoteConfigClient, maxRetries int, retryDelay time.Duration) Config {
	return Config{
		Site:           site,
		DDRegistries:   ddRegistries,
		RCClient:       rcClient,
		MaxInitRetries: maxRetries,
		InitRetryDelay: retryDelay,
	}
}
