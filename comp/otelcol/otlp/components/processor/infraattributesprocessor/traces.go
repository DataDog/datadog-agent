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
	logger      *zap.Logger
	tagger      taggerClient
	cardinality types.TagCardinality
	generateID  GenerateKubeMetadataEntityID
}

func newInfraAttributesSpanProcessor(set processor.Settings, cfg *Config, tagger taggerClient, generateID GenerateKubeMetadataEntityID) (*infraAttributesSpanProcessor, error) {
	iasp := &infraAttributesSpanProcessor{
		logger:      set.Logger,
		tagger:      tagger,
		cardinality: cfg.Cardinality,
		generateID:  generateID,
	}
	set.Logger.Info("Span Infra Attributes Processor configured")
	return iasp, nil
}

func (iasp *infraAttributesSpanProcessor) processTraces(_ context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	rss := td.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		resourceAttributes := rss.At(i).Resource().Attributes()
		processInfraTags(iasp.logger, iasp.tagger, iasp.cardinality, iasp.generateID, resourceAttributes)
	}
	return td, nil
}
