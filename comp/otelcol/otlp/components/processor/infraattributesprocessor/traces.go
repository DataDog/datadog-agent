// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	conventions "go.opentelemetry.io/collector/semconv/v1.21.0"
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
		entityIDs := entityIDsFromAttributes(resourceAttributes, iasp.generateID)
		tagMap := make(map[string]string)

		// Get all unique tags from resource attributes and global tags
		for _, entityID := range entityIDs {
			entityTags, err := iasp.tagger.Tag(entityID, iasp.cardinality)
			if err != nil {
				iasp.logger.Error("Cannot get tags for entity", zap.String("entityID", entityID.String()), zap.Error(err))
				continue
			}
			for _, tag := range entityTags {
				k, v := splitTag(tag)
				_, hasTag := tagMap[k]
				if k != "" && v != "" && !hasTag {
					tagMap[k] = v
				}
			}
		}
		globalTags, err := iasp.tagger.GlobalTags(iasp.cardinality)
		if err != nil {
			iasp.logger.Error("Cannot get global tags", zap.Error(err))
		}
		for _, tag := range globalTags {
			k, v := splitTag(tag)
			_, hasTag := tagMap[k]
			if k != "" && v != "" && !hasTag {
				tagMap[k] = v
			}
		}
		// Add all tags as resource attributes
		for k, v := range tagMap {
			// Add OTel semantics for universal service tags which are required in mapping
			if k == tags.Service {
				resourceAttributes.PutStr(conventions.AttributeServiceName, v)
				continue
			}
			if k == tags.Env {
				resourceAttributes.PutStr(conventions.AttributeDeploymentEnvironment, v)
				continue
			}
			if k == tags.Version {
				resourceAttributes.PutStr(conventions.AttributeServiceVersion, v)
				continue
			}
			resourceAttributes.PutStr(k, v)
		}
	}
	return td, nil
}
