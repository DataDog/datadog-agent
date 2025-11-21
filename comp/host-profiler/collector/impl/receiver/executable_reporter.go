// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package receiver implements the receiver for the host profiler.
package receiver

import (
	"context"

	"go.uber.org/zap"

	"github.com/DataDog/dd-otel-host-profiler/reporter"

	ebpfreporter "go.opentelemetry.io/ebpf-profiler/reporter"
)

var _ ebpfreporter.ExecutableReporter = (*executableReporter)(nil)

type executableReporter struct {
	symbolUploader *reporter.DatadogSymbolUploader
}

func newExecutableReporter(config *reporter.SymbolUploaderConfig, _ *zap.Logger) (*executableReporter, error) {
	ctx := context.Background()
	symbolUploader, err := reporter.NewDatadogSymbolUploader(ctx, config)
	if err != nil {
		return nil, err
	}

	symbolUploader.Start(ctx)
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
