// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package imageresolver

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

type RemoteConfigClient interface {
	GetConfigs(product string) map[string]state.RawConfig
	Subscribe(product string, callback func(map[string]state.RawConfig, func(string, state.ApplyStatus)))
}

type ImageResolverConfig struct {
	Site           string
	DDRegistries   map[string]any
	RCClient       RemoteConfigClient
	MaxInitRetries int
	InitRetryDelay time.Duration
}

func NewImageResolverConfig(cfg config.Component, rcClient RemoteConfigClient) *ImageResolverConfig {
	return &ImageResolverConfig{
		Site:           cfg.GetString("site"),
		DDRegistries:   cfg.GetStringMap("admission_controller.auto_instrumentation.default_dd_registries"),
		RCClient:       rcClient,
		MaxInitRetries: 5,
		InitRetryDelay: 1 * time.Second,
	}
}

func NewTestImageResolverConfig(site string, ddRegistries map[string]any, rcClient RemoteConfigClient, maxRetries int, retryDelay time.Duration) *ImageResolverConfig {
	return &ImageResolverConfig{
		Site:           site,
		DDRegistries:   ddRegistries,
		RCClient:       rcClient,
		MaxInitRetries: maxRetries,
		InitRetryDelay: retryDelay,
	}
}
