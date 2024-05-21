// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

type infraAttributesSpanProcessor struct {
	logger *zap.Logger
}

func newInfraAttributesSpanProcessor(set processor.CreateSettings, _ *Config) (*infraAttributesSpanProcessor, error) {
	tesp := &infraAttributesSpanProcessor{
		logger: set.Logger,
	}
	set.Logger.Info("Span Infra Attributes Processor configured")
	return tesp, nil
}

func (tesp *infraAttributesSpanProcessor) processTraces(_ context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	return td, nil
}
