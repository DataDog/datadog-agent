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
	// When metrics_attributes_as_tags is enabled, promote custom tagger labels (e.g. from
	// kubernetesResourcesLabelsAsTags) so they survive the metrics translator's
	// allowlist. The metrics path consumes them via the `datadog.container.tag.`
	// prefix: attributes.TagsFromAttributes (metrics_translator.go) calls
	// attributes.ContainerTagsFromResourceAttributes, which extracts that prefix
	// into metric tags. Without promotion, custom labels that are not known DD /
	// OTel conventions are dropped from OTLP metrics (see OTELS-1131). "duplicate"
	// keeps the original resource attribute and additionally writes the prefixed
	// form the translator reads.
	promote := ContainerTagPromotionOff
	if iamp.cfg.MetricsAttributesAsTags {
		promote = ContainerTagPromotionDuplicate
	}

	rms := md.ResourceMetrics()
	batch := iamp.infraTags.newTagBatch()
	for i := 0; i < rms.Len(); i++ {
		resourceAttributes := rms.At(i).Resource().Attributes()
		batch.ProcessTags(iamp.logger, iamp.cardinality, resourceAttributes, iamp.cfg.AllowHostnameOverride, promote, false, nil)
	}
	return md, nil
}
