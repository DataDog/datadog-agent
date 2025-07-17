// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

type infraAttributesSpanProcessor struct {
	infraTags   infraTagsProcessor
	logger      *zap.Logger
	cardinality types.TagCardinality
	cfg         *Config
}

func newInfraAttributesSpanProcessor(
	set processor.Settings,
	infraTags infraTagsProcessor,
	cfg *Config,
) (*infraAttributesSpanProcessor, error) {
	iasp := &infraAttributesSpanProcessor{
		infraTags:   infraTags,
		logger:      set.Logger,
		cardinality: cfg.Cardinality,
		cfg:         cfg,
	}
	set.Logger.Info("Span Infra Attributes Processor configured")
	return iasp, nil
}

func (iasp *infraAttributesSpanProcessor) processTraces(_ context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	rss := td.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		resourceAttributes := rss.At(i).Resource().Attributes()
		iasp.infraTags.ProcessTags(iasp.logger, iasp.cardinality, resourceAttributes, iasp.cfg.AllowHostnameOverride)
	}
	return td, nil
}
