// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

type infraAttributesMetricProcessor struct {
	infraTags   infraTagsProcessor
	logger      *zap.Logger
	cardinality types.TagCardinality
	cfg         *Config
}

func newInfraAttributesMetricProcessor(
	set processor.Settings,
	infraTags infraTagsProcessor,
	cfg *Config,
) (*infraAttributesMetricProcessor, error) {
	iamp := &infraAttributesMetricProcessor{
		infraTags:   infraTags,
		logger:      set.Logger,
		cardinality: cfg.Cardinality,
		cfg:         cfg,
	}
	set.Logger.Info("Metric Infra Attributes Processor configured")
	return iamp, nil
}

func (iamp *infraAttributesMetricProcessor) processMetrics(_ context.Context, md pmetric.Metrics) (pmetric.Metrics, error) {
	rms := md.ResourceMetrics()
	for i := 0; i < rms.Len(); i++ {
		resourceAttributes := rms.At(i).Resource().Attributes()
		iamp.infraTags.ProcessTags(iamp.logger, iamp.cardinality, resourceAttributes, iamp.cfg.AllowHostnameOverride)
	}
	return md, nil
}
