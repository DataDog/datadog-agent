// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"
)

var processorCapabilities = consumer.Capabilities{MutatesData: true}

// TODO: Remove tagger and generateID as depenendencies to enable future import of
// infraattributesprocessor by external go packages like ocb
type factory struct {
	tagger     taggerClient
	generateID GenerateKubeMetadataEntityID
}

// NewFactory returns a new factory for the InfraAttributes processor.
func NewFactory(tagger taggerClient, generateID GenerateKubeMetadataEntityID) processor.Factory {
	f := &factory{
		tagger:     tagger,
		generateID: generateID,
	}

	return processor.NewFactory(
		Type,
		f.createDefaultConfig,
		processor.WithMetrics(f.createMetricsProcessor, MetricsStability),
		processor.WithLogs(f.createLogsProcessor, LogsStability),
		processor.WithTraces(f.createTracesProcessor, TracesStability),
	)
}

func (f *factory) createDefaultConfig() component.Config {
	return &Config{
		Cardinality: types.LowCardinality,
	}
}

func (f *factory) createMetricsProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	nextConsumer consumer.Metrics,
) (processor.Metrics, error) {
	iap, err := newInfraAttributesMetricProcessor(set, cfg.(*Config), f.tagger, f.generateID)
	if err != nil {
		return nil, err
	}
	return processorhelper.NewMetricsProcessor(
		ctx,
		set,
		cfg,
		nextConsumer,
		iap.processMetrics,
		processorhelper.WithCapabilities(processorCapabilities))
}

func (f *factory) createLogsProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	nextConsumer consumer.Logs,
) (processor.Logs, error) {
	iap, err := newInfraAttributesLogsProcessor(set, cfg.(*Config), f.tagger, f.generateID)
	if err != nil {
		return nil, err
	}
	return processorhelper.NewLogsProcessor(
		ctx,
		set,
		cfg,
		nextConsumer,
		iap.processLogs,
		processorhelper.WithCapabilities(processorCapabilities))
}

func (f *factory) createTracesProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	nextConsumer consumer.Traces,
) (processor.Traces, error) {
	iap, err := newInfraAttributesSpanProcessor(set, cfg.(*Config), f.tagger, f.generateID)
	if err != nil {
		return nil, err
	}
	return processorhelper.NewTracesProcessor(
		ctx,
		set,
		cfg,
		nextConsumer,
		iap.processTraces,
		processorhelper.WithCapabilities(processorCapabilities))
}
