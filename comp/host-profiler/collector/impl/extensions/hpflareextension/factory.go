// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package hpflareextension defines the OpenTelemetry Extension implementation.
package hpflareextension

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/extension"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
)

const (
	defaultHTTPPort = 7778
)

type ddExtensionFactory struct {
	extension.Factory
	ipcComp ipc.Component
}

// NewFactoryForAgent creates a factory for Datadog Flare Extension for use with Agent
func NewFactoryForAgent(ipcComp ipc.Component) extension.Factory {
	return &ddExtensionFactory{
		ipcComp: ipcComp,
	}
}

// Create creates a new instance of the Datadog Flare Extension
func (f *ddExtensionFactory) Create(_ context.Context, set extension.Settings, cfg component.Config) (extension.Extension, error) {
	config := &Config{}
	config.HTTPConfig = cfg.(*Config).HTTPConfig
	return NewExtension(config, f.ipcComp, set.TelemetrySettings)
}

func (f *ddExtensionFactory) CreateDefaultConfig() component.Config {
	return &Config{
		HTTPConfig: &confighttp.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  fmt.Sprintf("localhost:%d", defaultHTTPPort),
				Transport: confignet.TransportTypeTCP,
			},
		},
	}
}

func (f *ddExtensionFactory) Type() component.Type {
	return component.MustNewType("hpflare")
}

// Stability returns the stability level of the component
func (f *ddExtensionFactory) Stability() component.StabilityLevel {
	return component.StabilityLevelDevelopment
}
