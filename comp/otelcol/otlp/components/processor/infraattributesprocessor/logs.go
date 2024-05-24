// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"

	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
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
	telp := &infraAttributesLogProcessor{
		logger:      set.Logger,
		tagger:      tagger,
		cardinality: cfg.Cardinality,
	}

	set.Logger.Info("Logs Infra Attributes configured")
	return telp, nil
}

func (ialp *infraAttributesLogProcessor) processLogs(_ context.Context, ld plog.Logs) (plog.Logs, error) {
	rls := ld.ResourceLogs()
	for i := 0; i < rls.Len(); i++ {
		rl := rls.At(i)
		rattrs := rl.Resource().Attributes()
		originID := attributes.OriginIDFromAttributes(rattrs)

		entityTags, err := ialp.tagger.Tag(originID, ialp.cardinality)
		if err != nil {
			ialp.logger.Error("Cannot get tags for entity", zap.String("originID", originID), zap.Error(err))
			continue
		}

		globalTags, err := ialp.tagger.GlobalTags(ialp.cardinality)
		if err != nil {
			ialp.logger.Error("Cannot get global tags", zap.Error(err))
			continue
		}

		enrichedTags := make([]string, 0, len(entityTags)+len(globalTags))
		enrichedTags = append(enrichedTags, entityTags...)
		enrichedTags = append(enrichedTags, globalTags...)
		for _, tag := range enrichedTags {
			k, v := splitTag(tag)
			if k != "" && v != "" {
				rattrs.PutStr(k, v)
			}
		}
	}

	return ld, nil
}
