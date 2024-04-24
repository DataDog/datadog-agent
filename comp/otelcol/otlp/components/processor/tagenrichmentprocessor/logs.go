// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package tagenrichmentprocessor

import (
	"context"

	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

type tagEnrichmentLogProcessor struct {
	logger    *zap.Logger
}

func newTagEnrichmentLogsProcessor(set processor.CreateSettings, cfg *Config) (*tagEnrichmentLogProcessor, error) {
	telp := &tagEnrichmentLogProcessor{
		logger: set.Logger,
	}

	set.Logger.Info("Logs Tag Enrichment configured")
	return telp, nil
}

func (telp *tagEnrichmentLogProcessor) processLogs(ctx context.Context, ld plog.Logs) (plog.Logs, error) {
	return ld, nil
}
