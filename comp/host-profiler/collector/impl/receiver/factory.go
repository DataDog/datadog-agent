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
	"go.opentelemetry.io/ebpf-profiler/tracer"
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
	var options []ebpfcollector.Option
	var shutdowns []func() error

	if config.SymbolUploader.Enabled {
		executableReporter, err := newExecutableReporter(&config.SymbolUploader, logger)
		if err != nil {
			return nil, err
		}

		options = append(options, ebpfcollector.WithExecutableReporter(executableReporter))
		shutdowns = append(shutdowns, executableReporter.Stop)
	}

	var mp *memoryProfiler
	mpCtx := ctx
	if config.MemoryProfiling.Enabled {
		// Buffered so the WithTracerAccess callback never blocks on mp.Start().
		tracerCh := make(chan *tracer.Tracer, 1)

		// Create context before the goroutine so mp.cancel is set before Shutdown() can run.
		var mpCancel context.CancelFunc
		mpCtx, mpCancel = context.WithCancel(ctx)

		memProfConfig := config.MemoryProfiling
		options = append(options, ebpfcollector.WithTracerAccess(func(t *tracer.Tracer) {
			tracerCh <- t

			// SetPIDNewCallback fires on the PID event processor goroutine; dispatch
			// to avoid blocking it.
			t.SetPIDNewCallback(func(pid int) {
				go func() {
					if shouldProfileProcess(pid, memProfConfig) {
						if err := t.AttachMemoryProfilingForPID(pid); err != nil {
							logger.Debug(fmt.Sprintf("memory profiling attach for new PID %d: %v", pid, err))
						}
					}
				}()
			})
		}))

		options = append(options,
			ebpfcollector.WithMemoryProfiling(true, config.MemoryProfiling.AllocationSampleRate))

		mp = newMemoryProfiler(config.MemoryProfiling, logger, tracerCh, mpCancel)
		shutdowns = append(shutdowns, mp.Shutdown)

		logger.Info("Memory profiling enabled")
	}

	if len(shutdowns) > 0 {
		fns := shutdowns
		options = append(options, ebpfcollector.WithOnShutdown(func() error {
			var firstErr error
			for _, fn := range fns {
				if err := fn(); err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		}))
	}

	createProfiles = ebpfcollector.BuildProfilesReceiver(options...)

	receiver, err := createProfiles(
		ctx,
		rs,
		config.EbpfCollectorConfig,
		nextConsumer)
	if err != nil {
		return nil, err
	}

	// mp.Start blocks until the tracer arrives (sent during receiver.Start() via WithTracerAccess).
	// wg.Add(1) before the goroutine so Shutdown()'s wg.Wait() is race-free.
	if mp != nil {
		mp.wg.Add(1)
		go mp.Start(mpCtx)
	}

	return receiver, nil
}
