// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"

	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

type infraAttributesMetricProcessor struct {
	logger      *zap.Logger
	cardinality types.TagCardinality
}

func newInfraAttributesMetricProcessor(set processor.CreateSettings, cfg *Config) (*infraAttributesMetricProcessor, error) {
	tesp := &infraAttributesMetricProcessor{
		logger:      set.Logger,
		cardinality: cfg.Cardinality,
	}
	set.Logger.Info("Metric Tag Enrichment configured")
	return tesp, nil
}

func splitTag(tag string) (key string, value string) {
	split := strings.SplitN(tag, ":", 2)
	if len(split) < 2 || split[0] == "" || split[1] == "" {
		return "", ""
	}
	return split[0], split[1]
}

func (temp *infraAttributesMetricProcessor) processMetrics(_ context.Context, md pmetric.Metrics) (pmetric.Metrics, error) {
	rms := md.ResourceMetrics()
	for i := 0; i < rms.Len(); i++ {
		rm := rms.At(i)
		rattrs := rm.Resource().Attributes()
		originId := attributes.OriginIDFromAttributes(rattrs)

		entityTags, err := tagger.Tag(originId, temp.cardinality)
		if err != nil {
			temp.logger.Error("Cannot get tags for entity", zap.String("originId", originId), zap.Error(err))
			continue
		}

		globalTags, err := tagger.GlobalTags(temp.cardinality)
		if err != nil {
			temp.logger.Error("Cannot get global tags", zap.Error(err))
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

	return md, nil
}
