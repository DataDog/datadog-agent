// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializerexporter

import (
	"context"
	"io"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	otlpmetrics "github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/metrics"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/quantile"
)

var _ otlpmetrics.Consumer = (*serializerConsumer)(nil)

type serializerConsumer struct {
	cardinality collectors.TagCardinality
	extraTags   []string
	series      metrics.Series
	sketches    metrics.SketchSeriesList
	apmstats    []io.Reader
}

// enrichedTags of a given dimension.
// In the OTLP pipeline, 'contexts' are kept within the translator and function differently than DogStatsD/check metrics.
func (c *serializerConsumer) enrichedTags(dimensions *otlpmetrics.Dimensions) []string {
	panic("not called")
}

func (c *serializerConsumer) ConsumeAPMStats(ss *pb.ClientStatsPayload) {
	panic("not called")
}

func (c *serializerConsumer) ConsumeSketch(_ context.Context, dimensions *otlpmetrics.Dimensions, ts uint64, qsketch *quantile.Sketch) {
	panic("not called")
}

func apiTypeFromTranslatorType(typ otlpmetrics.DataType) metrics.APIMetricType {
	panic("not called")
}

func (c *serializerConsumer) ConsumeTimeSeries(ctx context.Context, dimensions *otlpmetrics.Dimensions, typ otlpmetrics.DataType, ts uint64, value float64) {
	panic("not called")
}

// addTelemetryMetric to know if an Agent is using OTLP metrics.
func (c *serializerConsumer) addTelemetryMetric(hostname string) {
	panic("not called")
}

// addRuntimeTelemetryMetric to know if an Agent is using OTLP runtime metrics.
func (c *serializerConsumer) addRuntimeTelemetryMetric(hostname string, languageTags []string) {
	panic("not called")
}

// Send exports all data recorded by the consumer. It does not reset the consumer.
func (c *serializerConsumer) Send(s serializer.MetricSerializer) error {
	panic("not called")
}

func (c *serializerConsumer) sendAPMStats() error {
	panic("not called")
}
