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

	ebpfreporter "go.opentelemetry.io/ebpf-profiler/reporter"

	"github.com/DataDog/datadog-agent/comp/host-profiler/symboluploader"
)

var _ ebpfreporter.ExecutableReporter = (*executableReporter)(nil)

type executableReporter struct {
	symbolUploader *symboluploader.DatadogSymbolUploader
}

func newExecutableReporter(config *symboluploader.SymbolUploaderConfig, _ *zap.Logger) (*executableReporter, error) {
	ctx := context.Background()
	symbolUploader, err := symboluploader.NewDatadogSymbolUploader(ctx, config)
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
