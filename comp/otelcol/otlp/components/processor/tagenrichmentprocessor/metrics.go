// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package tagenrichmentprocessor

import (
	"context"

	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

type tagEnrichmentMetricProcessor struct {
	logger            *zap.Logger
}

func newTagEnrichmentMetricProcessor(set processor.CreateSettings, cfg *Config) (*tagEnrichmentMetricProcessor, error) {
	tesp := &tagEnrichmentMetricProcessor{
		logger: set.Logger,
	}
	set.Logger.Info("Metric Tag Enrichment configured")
	return tesp, nil
}

func (temp *tagEnrichmentMetricProcessor) processMetrics(ctx context.Context, md pmetric.Metrics) (pmetric.Metrics, error) {
	return md, nil
}


