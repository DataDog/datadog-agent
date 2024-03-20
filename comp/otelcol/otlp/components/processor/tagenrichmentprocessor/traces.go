// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package tagenrichmentprocessor

import (
	"context"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

type tagEnrichmentSpanProcessor struct {
	logger            *zap.Logger
}

func newTagEnrichmentSpanProcessor(set processor.CreateSettings, cfg *Config) (*tagEnrichmentSpanProcessor, error) {
	tesp := &tagEnrichmentSpanProcessor{
		logger: set.Logger,
	}
	set.Logger.Info("Span Tag Enrichment configured")
	return tesp, nil
}

func (tesp *tagEnrichmentSpanProcessor) processTraces(ctx context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	return td, nil
}
