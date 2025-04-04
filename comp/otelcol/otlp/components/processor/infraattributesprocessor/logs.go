// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

type infraAttributesLogProcessor struct {
	infraTags   infraTagsProcessor
	logger      *zap.Logger
	cardinality types.TagCardinality
	cfg         *Config
}

func newInfraAttributesLogsProcessor(
	set processor.Settings,
	infraTags infraTagsProcessor,
	cfg *Config,
) (*infraAttributesLogProcessor, error) {
	ialp := &infraAttributesLogProcessor{
		infraTags:   infraTags,
		logger:      set.Logger,
		cardinality: cfg.Cardinality,
		cfg:         cfg,
	}
	set.Logger.Info("Logs Infra Attributes Processor configured")
	return ialp, nil
}

func (ialp *infraAttributesLogProcessor) processLogs(_ context.Context, ld plog.Logs) (plog.Logs, error) {
	rls := ld.ResourceLogs()
	for i := 0; i < rls.Len(); i++ {
		resourceAttributes := rls.At(i).Resource().Attributes()
		ialp.infraTags.ProcessTags(ialp.logger, ialp.cardinality, resourceAttributes, ialp.cfg.AllowHostnameOverride)
	}
	return ld, nil
}
