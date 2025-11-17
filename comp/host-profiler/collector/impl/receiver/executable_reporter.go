// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package receiver implements the receiver for the host profiler.
package receiver

import (
	"context"
	"fmt"
	"os"

	"go.uber.org/zap"

	log "github.com/DataDog/datadog-agent/pkg/util/log/zap"
	"github.com/DataDog/dd-otel-host-profiler/reporter"
	"github.com/DataDog/dd-otel-host-profiler/runner"

	ebpfreporter "go.opentelemetry.io/ebpf-profiler/reporter"
)

var _ ebpfreporter.ExecutableReporter = (*executableReporter)(nil)

type executableReporter struct {
	symbolUploader *reporter.DatadogSymbolUploader
}

func newExecutableReporter(config *reporter.SymbolUploaderConfig) (*executableReporter, error) {
	// Wrap zap.Logger to implement the Logger interface expected by the reporter
	wrappedLogger := newLogger()

	config.SymbolEndpoints = runner.GetValidSymbolEndpoints(
		os.Getenv("DD_SITE"), os.Getenv("DD_API_KEY"), os.Getenv("DD_APP_KEY"),
		config.SymbolEndpoints,
		func(msg string) { wrappedLogger.Infof(msg) }, func(msg string) { wrappedLogger.Warnf(msg) })

	symbolUploader, err := reporter.NewDatadogSymbolUploader(config, wrappedLogger)
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

// zapLoggerWrapper wraps a zap.Logger to implement the Logger interface
type zapLoggerWrapper struct {
	logger *zap.Logger
}

// newLogger creates a Logger from a zap.Logger
func newLogger() reporter.Logger {
	// Add 1 to the call stack depth to skip the newLogger function
	return &zapLoggerWrapper{logger: zap.New(log.NewZapCoreWithRelativeDepth(1))}
}

func (l *zapLoggerWrapper) Debugf(format string, args ...interface{}) {
	if l.logger.Level() <= zap.DebugLevel {
		l.logger.Debug(fmt.Sprintf(format, args...))
	}
}

func (l *zapLoggerWrapper) Infof(format string, args ...interface{}) {
	if l.logger.Level() <= zap.InfoLevel {
		l.logger.Info(fmt.Sprintf(format, args...))
	}
}

func (l *zapLoggerWrapper) Warnf(format string, args ...interface{}) {
	if l.logger.Level() <= zap.WarnLevel {
		l.logger.Warn(fmt.Sprintf(format, args...))
	}
}

func (l *zapLoggerWrapper) Errorf(format string, args ...interface{}) {
	if l.logger.Level() <= zap.ErrorLevel {
		l.logger.Error(fmt.Sprintf(format, args...))
	}
}
