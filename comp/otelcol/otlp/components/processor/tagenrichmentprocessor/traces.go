// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package tagenrichmentprocessor

import (
	"context"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

type tagEnrichmentSpanProcessor struct {
	logger *zap.Logger
}

func newTagEnrichmentSpanProcessor(set processor.CreateSettings, _ *Config) (*tagEnrichmentSpanProcessor, error) {
	tesp := &tagEnrichmentSpanProcessor{
		logger: set.Logger,
	}
	set.Logger.Info("Span Tag Enrichment configured")
	return tesp, nil
}

func (tesp *tagEnrichmentSpanProcessor) processTraces(_ context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	return td, nil
}
