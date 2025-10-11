// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package receiver implements the receiver for the host profiler.
package receiver

import (
	"context"
	"os"

	"github.com/DataDog/dd-otel-host-profiler/reporter"
	ebpfreporter "go.opentelemetry.io/ebpf-profiler/reporter"
)

var _ ebpfreporter.ExecutableReporter = (*executableReporter)(nil)

type executableReporter struct {
	symbolUploader *reporter.DatadogSymbolUploader
}

func newExecutableReporter(config *reporter.SymbolUploaderConfig) (*executableReporter, error) {
	// TODO: Use the same logic as https://github.com/DataDog/dd-otel-host-profiler/blob/0b49a0b150a52d450612688e5d0be05c47336128/runner/runner.go#L206-L224
	// for SymbolEndpoints.
	if len(config.SymbolEndpoints) == 0 {
		config.SymbolEndpoints = []reporter.SymbolEndpoint{
			{
				APIKey: os.Getenv("DD_API_KEY"),
				AppKey: os.Getenv("DD_APP_KEY"),
				Site:   os.Getenv("DD_SITE"),
			},
		}
	}
	symbolUploader, err := reporter.NewDatadogSymbolUploader(config)
	if err != nil {
		return nil, err
	}

	symbolUploader.Start(context.Background())
	return &executableReporter{
		symbolUploader: symbolUploader,
	}, nil
}

func (m *executableReporter) Stop() error {
	m.symbolUploader.Stop()
	return nil
}

func (m *executableReporter) ReportExecutable(args *ebpfreporter.ExecutableMetadata) {
	m.symbolUploader.UploadSymbols(args)
}
