// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package receiver

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/dd-otel-host-profiler/reporter"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/xconsumer"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/xreceiver"
	ebpfcollector "go.opentelemetry.io/ebpf-profiler/collector"
)

// GetFactoryName returns the name of the factory.
func GetFactoryName() string {
	return "hostprofiler"
}

// NewFactory creates a factory for the receiver.
func NewFactory() receiver.Factory {
	return xreceiver.NewFactory(
		component.MustNewType(GetFactoryName()),
		defaultConfig,
		xreceiver.WithProfiles(createProfilesReceiver, component.StabilityLevelAlpha))
}

func createProfilesReceiver(
	ctx context.Context,
	rs receiver.Settings,
	baseCfg component.Config,
	nextConsumer xconsumer.Profiles) (xreceiver.Profiles, error) {
	config, ok := baseCfg.(Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type. Expected %T, got %T", Config{}, baseCfg)
	}

	var createProfiles xreceiver.CreateProfilesFunc
	if config.SymbolUploader.Enabled {
		executableReporter, err := newExecutableReporter(&config.SymbolUploader)
		if err != nil {
			return nil, err
		}

		createProfiles = ebpfcollector.BuildProfilesReceiver(
			ebpfcollector.WithExecutableReporter(executableReporter),
			ebpfcollector.WithOnShutdown(executableReporter.Stop))
	} else {
		createProfiles = ebpfcollector.BuildProfilesReceiver()
	}

	return createProfiles(
		ctx,
		rs,
		config.Config,
		nextConsumer)
}

// Config is the configuration for the profiles receiver.
type Config struct {
	*ebpfcollector.Config   `mapstructure:",squash"`
	SymbolUploader          reporter.SymbolUploaderConfig `mapstructure:"symbol_uploader"`
	EnableGoRuntimeProfiler bool                          `mapstructure:"enable_go_runtime_profiler"`
}

// GetDefaultEnableGoRuntimeProfiler returns the default value for the enable_go_runtime_profiler config.
func GetDefaultEnableGoRuntimeProfiler() bool {
	return false // TODO use constant in dd-otel-host-profiler when available
}

func defaultConfig() component.Config {
	cfg := ebpfcollector.NewFactory().CreateDefaultConfig().(*ebpfcollector.Config)

	return Config{
		Config: cfg,

		// TODO: This is a temporary config, it will be updated to use the same config values
		// as dd-otel-host-profiler in a later PR.
		SymbolUploader: reporter.SymbolUploaderConfig{
			Enabled:                        true,
			UploadDynamicSymbols:           false,
			UploadGoPCLnTab:                true,
			UseHTTP2:                       false,
			SymbolQueryInterval:            time.Second * 5,
			DisableDebugSectionCompression: false,
			DryRun:                         false,
			SymbolEndpoints:                nil,
			Version:                        "0.0.0",
		},

		EnableGoRuntimeProfiler: GetDefaultEnableGoRuntimeProfiler(),
	}
}
