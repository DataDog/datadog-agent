// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package receiver

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/xconsumer"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/xreceiver"
	ebpfcollector "go.opentelemetry.io/ebpf-profiler/collector"
)

// NewFactory creates a factory for the receiver.
func NewFactory() receiver.Factory {
	return xreceiver.NewFactory(
		component.MustNewType("hostprofiler"),
		defaultConfig,
		xreceiver.WithProfiles(createProfilesReceiver, component.StabilityLevelAlpha))
}

func createProfilesReceiver(
	ctx context.Context,
	rs receiver.Settings,
	baseCfg component.Config,
	nextConsumer xconsumer.Profiles) (xreceiver.Profiles, error) {
	logger := rs.Logger
	config, ok := baseCfg.(Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type. Expected %T, got %T", Config{}, baseCfg)
	}

	logger.Info("Enabled tracers: " + config.EbpfCollectorConfig.Tracers)

	var createProfiles xreceiver.CreateProfilesFunc
	if config.SymbolUploader.Enabled {
		executableReporter, err := newExecutableReporter(&config.SymbolUploader, logger)
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
		config.EbpfCollectorConfig,
		nextConsumer)
}
