// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"

	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

type infraAttributesLogProcessor struct {
	logger      *zap.Logger
	tagger      tagger.Component
	cardinality types.TagCardinality
}

func newInfraAttributesLogsProcessor(set processor.CreateSettings, cfg *Config, tagger tagger.Component) (*infraAttributesLogProcessor, error) {
	ialp := &infraAttributesLogProcessor{
		logger:      set.Logger,
		tagger:      tagger,
		cardinality: cfg.Cardinality,
	}

	set.Logger.Info("Logs Infra Attributes Processor configured")
	return ialp, nil
}

func (ialp *infraAttributesLogProcessor) processLogs(_ context.Context, ld plog.Logs) (plog.Logs, error) {
	rls := ld.ResourceLogs()
	for i := 0; i < rls.Len(); i++ {
		resourceAttributes := rls.At(i).Resource().Attributes()
		entityIDs := entityIDsFromAttributes(resourceAttributes)
		tagMap := make(map[string]string)

		// Get all unique tags from resource attributes and global tags
		for _, entityID := range entityIDs {
			entityTags, err := ialp.tagger.Tag(entityID, ialp.cardinality)
			if err != nil {
				ialp.logger.Error("Cannot get tags for entity", zap.String("entityID", entityID), zap.Error(err))
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
		globalTags, err := ialp.tagger.GlobalTags(ialp.cardinality)
		if err != nil {
			ialp.logger.Error("Cannot get global tags", zap.Error(err))
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
			resourceAttributes.PutStr(k, v)
		}
	}
	return ld, nil
}
