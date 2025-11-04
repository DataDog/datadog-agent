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

	"go.uber.org/zap"

	"github.com/DataDog/dd-otel-host-profiler/reporter"
	"github.com/DataDog/dd-otel-host-profiler/runner"

	ebpfreporter "go.opentelemetry.io/ebpf-profiler/reporter"
)

var _ ebpfreporter.ExecutableReporter = (*executableReporter)(nil)

type executableReporter struct {
	symbolUploader *reporter.DatadogSymbolUploader
}

func newExecutableReporter(config *reporter.SymbolUploaderConfig, logger *zap.Logger) (*executableReporter, error) {
	config.SymbolEndpoints = runner.GetValidSymbolEndpoints(
		os.Getenv("DD_SITE"), os.Getenv("DD_API_KEY"), os.Getenv("DD_APP_KEY"),
		config.SymbolEndpoints,
		func(msg string) { logger.Info(msg) }, func(msg string) { logger.Warn(msg) })

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
