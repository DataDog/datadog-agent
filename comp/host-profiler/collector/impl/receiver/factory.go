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
	"go.opentelemetry.io/ebpf-profiler/reporter"
)

// NewFactory creates a factory for the profiling receiver.
// crash is optional: when provided, crash-origin events are routed to it
// instead of the regular batch reporter.
func NewFactory(profilerName string, crash reporter.Reporter) receiver.Factory {
	return xreceiver.NewFactory(
		component.MustNewType("profiling"),
		func() component.Config { return defaultConfig(profilerName) },
		xreceiver.WithProfiles(makeCreateFunc(crash), component.StabilityLevelAlpha))
}

func makeCreateFunc(crash reporter.Reporter) xreceiver.CreateProfilesFunc {
	return func(ctx context.Context, rs receiver.Settings, baseCfg component.Config, nextConsumer xconsumer.Profiles) (xreceiver.Profiles, error) {
		return createProfilesReceiver(ctx, rs, baseCfg, nextConsumer, crash)
	}
}

func createProfilesReceiver(
	ctx context.Context,
	rs receiver.Settings,
	baseCfg component.Config,
	nextConsumer xconsumer.Profiles,
	crash reporter.Reporter,
) (xreceiver.Profiles, error) {
	logger := rs.Logger
	config, ok := baseCfg.(Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type. Expected %T, got %T", Config{}, baseCfg)
	}

	logger.Info("Enabled tracers: " + config.EbpfCollectorConfig.Tracers)

	opts := []ebpfcollector.Option{}
	if config.SymbolUploader.Enabled {
		executableReporter, err := newExecutableReporter(&config.SymbolUploader, logger)
		if err != nil {
			return nil, err
		}
		opts = append(opts,
			ebpfcollector.WithExecutableReporter(executableReporter),
			ebpfcollector.WithOnShutdown(executableReporter.Stop))
	}
	if crash != nil {
		opts = append(opts, WithCrashReporter(crash))
		config.EbpfCollectorConfig.CrashTracing = true
		logger.Info("Crash tracing enabled: OOM kills and fatal signals will be captured")
	}

	return ebpfcollector.BuildProfilesReceiver(opts...)(ctx, rs, config.EbpfCollectorConfig, nextConsumer)
}
