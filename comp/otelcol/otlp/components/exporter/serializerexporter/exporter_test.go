// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build test

package serializerexporter

import (
	"context"
	"strings"
	"testing"

	pkgdatadog "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/featuregate"
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/otel"
)

var _ serializer.MetricSerializer = (*metricRecorder)(nil)

type metricRecorder struct {
	serializer.Serializer // embed for implementing serializer.MetricSerializer

	sketchSeriesList metrics.SketchSeriesList
	series           []*metrics.Serie
}

// SendSketch implements the MetricSerializer interface
func (r *metricRecorder) SendSketch(s metrics.SketchesSource) error {
	for s.MoveNext() {
		c := s.Current()
		if c == nil {
			continue
		}
		r.sketchSeriesList = append(r.sketchSeriesList, c)
	}
	return nil
}

// SendIterableSeries implements the MetricSerializer interface
func (r *metricRecorder) SendIterableSeries(s metrics.SerieSource) error {
	for s.MoveNext() {
		c := s.Current()
		if c == nil {
			continue
		}
		r.series = append(r.series, c)
	}
	return nil
}

const (
	histogramMetricName        = "test.histogram"
	numberMetricName           = "test.gauge"
	histogramRuntimeMetricName = "process.runtime.dotnet.exceptions.count"
	numberRuntimeMetricName    = "process.runtime.go.goroutines"
)

func Test_ConsumeMetrics_Tags(t *testing.T) {
	tests := []struct {
		name                               string
		genMetrics                         func(t *testing.T) pmetric.Metrics
		wantSketchTags                     tagset.CompositeTags
		wantSerieTags                      tagset.CompositeTags
		extraTags                          []string
		instrumentationScopeMetadataAsTags bool
	}{
		{
			name: "no tags",
			genMetrics: func(_ *testing.T) pmetric.Metrics {
				h := pmetric.NewHistogramDataPoint()
				h.BucketCounts().FromRaw([]uint64{100})
				h.SetCount(100)
				h.SetSum(0)

				n := pmetric.NewNumberDataPoint()
				n.SetIntValue(777)
				return newMetrics(histogramMetricName, h, numberMetricName, n)
			},
			extraTags:      []string{},
			wantSketchTags: tagset.CompositeTagsFromSlice([]string{}),
			wantSerieTags:  tagset.CompositeTagsFromSlice([]string{}),
		},
		{
			name: "metric tags and extra tags",
			genMetrics: func(_ *testing.T) pmetric.Metrics {
				h := pmetric.NewHistogramDataPoint()
				h.BucketCounts().FromRaw([]uint64{100})
				h.SetCount(100)
				h.SetSum(0)
				hAttrs := h.Attributes()
				hAttrs.PutStr("histogram_1_id", "value1")
				hAttrs.PutStr("histogram_2_id", "value2")
				hAttrs.PutStr("histogram_3_id", "value3")

				n := pmetric.NewNumberDataPoint()
				n.SetIntValue(777)
				nAttrs := n.Attributes()
				nAttrs.PutStr("gauge_1_id", "value1")
				nAttrs.PutStr("gauge_2_id", "value2")
				nAttrs.PutStr("gauge_3_id", "value3")
				return newMetrics(histogramMetricName, h, numberMetricName, n)
			},
			extraTags: []string{"serverless_tag1:test1", "serverless_tag2:test2", "serverless_tag3:test3"},
			wantSketchTags: tagset.CompositeTagsFromSlice(
				[]string{
					"serverless_tag1:test1", "serverless_tag2:test2", "serverless_tag3:test3",
					"histogram_1_id:value1", "histogram_2_id:value2", "histogram_3_id:value3",
				},
			),
			wantSerieTags: tagset.CompositeTagsFromSlice(
				[]string{
					"serverless_tag1:test1", "serverless_tag2:test2", "serverless_tag3:test3",
					"gauge_1_id:value1", "gauge_2_id:value2", "gauge_3_id:value3",
				},
			),
		},
		{
			name: "runtime metrics, no tags",
			genMetrics: func(_ *testing.T) pmetric.Metrics {
				h := pmetric.NewHistogramDataPoint()
				h.BucketCounts().FromRaw([]uint64{100})
				h.SetCount(100)
				h.SetSum(0)

				n := pmetric.NewNumberDataPoint()
				n.SetIntValue(777)
				return newMetrics(histogramMetricName, h, numberMetricName, n)
			},
			wantSketchTags: tagset.CompositeTagsFromSlice([]string{}),
			wantSerieTags:  tagset.CompositeTagsFromSlice([]string{}),
		},
		{
			name: "runtime metrics, metric tags and extra tags",
			genMetrics: func(_ *testing.T) pmetric.Metrics {
				h := pmetric.NewHistogramDataPoint()
				h.BucketCounts().FromRaw([]uint64{100})
				h.SetCount(100)
				h.SetSum(0)
				hAttrs := h.Attributes()
				hAttrs.PutStr("histogram_1_id", "value1")
				hAttrs.PutStr("histogram_2_id", "value2")
				hAttrs.PutStr("histogram_3_id", "value3")

				n := pmetric.NewNumberDataPoint()
				n.SetIntValue(777)
				nAttrs := n.Attributes()
				nAttrs.PutStr("gauge_1_id", "value1")
				nAttrs.PutStr("gauge_2_id", "value2")
				nAttrs.PutStr("gauge_3_id", "value3")
				return newMetrics(histogramRuntimeMetricName, h, numberRuntimeMetricName, n)
			},
			extraTags: []string{"serverless_tag1:test1", "serverless_tag2:test2", "serverless_tag3:test3"},
			wantSketchTags: tagset.CompositeTagsFromSlice(
				[]string{
					"serverless_tag1:test1", "serverless_tag2:test2", "serverless_tag3:test3",
					"histogram_1_id:value1", "histogram_2_id:value2", "histogram_3_id:value3",
				},
			),
			wantSerieTags: tagset.CompositeTagsFromSlice(
				[]string{
					"serverless_tag1:test1", "serverless_tag2:test2", "serverless_tag3:test3",
					"gauge_1_id:value1", "gauge_2_id:value2", "gauge_3_id:value3",
				},
			),
		},
		{
			name: "instrumentation scope metadata as tags",
			genMetrics: func(_ *testing.T) pmetric.Metrics {
				h := pmetric.NewHistogramDataPoint()
				h.BucketCounts().FromRaw([]uint64{100})
				h.SetCount(100)
				h.SetSum(0)

				n := pmetric.NewNumberDataPoint()
				n.SetIntValue(777)
				md := newMetrics(histogramMetricName, h, numberMetricName, n)
				scope := md.ResourceMetrics().At(0).ScopeMetrics().At(0).Scope()
				scope.SetName("my_library")
				scope.SetVersion("v1.0.0")
				return md
			},
			extraTags: []string{},
			wantSketchTags: tagset.CompositeTagsFromSlice([]string{
				"instrumentation_scope:my_library", "instrumentation_scope_version:v1.0.0",
			}),
			wantSerieTags: tagset.CompositeTagsFromSlice([]string{
				"instrumentation_scope:my_library", "instrumentation_scope_version:v1.0.0",
			}),
			instrumentationScopeMetadataAsTags: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &metricRecorder{}
			ctx := context.Background()
			f := NewFactoryForOTelAgent(rec, &MockTagEnricher{}, func(context.Context) (string, error) {
				return "", nil
			}, nil, otel.NewDisabledGatewayUsage(), TelemetryStore{})
			cfg := f.CreateDefaultConfig().(*ExporterConfig)
			cfg.Metrics.Metrics.ExporterConfig.InstrumentationScopeMetadataAsTags = tt.instrumentationScopeMetadataAsTags
			cfg.Metrics.Tags = strings.Join(tt.extraTags, ",")
			exp, err := f.CreateMetrics(
				ctx,
				exportertest.NewNopSettings(component.MustNewType("datadog")),
				cfg,
			)
			require.NoError(t, err)
			require.NoError(t, exp.Start(ctx, componenttest.NewNopHost()))
			require.NoError(t, exp.ConsumeMetrics(ctx, tt.genMetrics(t)))
			require.NoError(t, exp.Shutdown(ctx))

			if tt.wantSketchTags.Len() > 0 {
				assert.Equal(t, tt.wantSketchTags, rec.sketchSeriesList[0].Tags)
			} else {
				assert.Equal(t, tagset.CompositeTagsFromSlice([]string{}), rec.sketchSeriesList[0].Tags)
			}
			assert.True(t, len(rec.series) > 0)
			for _, s := range rec.series {
				if s.Name == "datadog.agent.otlp.metrics" {
					assert.Equal(t, tagset.CompositeTagsFromSlice([]string{}), s.Tags)
				}
				if s.Name == "datadog.agent.otlp.runtime_metrics" {
					assert.True(t, s.Tags.Find(func(tag string) bool {
						return tag == "language:go" || tag == "language:dotnet"
					}))
				}
				if s.Name == numberMetricName {
					if tt.wantSerieTags.Len() > 0 {
						assert.Equal(t, tt.wantSerieTags, s.Tags)
					} else {
						assert.Equal(t, tagset.CompositeTagsFromSlice([]string{}), s.Tags)
					}
				}
			}
		})
	}
}

func Test_ConsumeMetrics_MetricOrigins(t *testing.T) {
	tests := []struct {
		name       string
		genMetrics func(t *testing.T) pmetric.Metrics
		msrc       metrics.MetricSource
	}{
		{
			name: "metric origin in sketches",
			genMetrics: func(_ *testing.T) pmetric.Metrics {
				md := pmetric.NewMetrics()
				rms := md.ResourceMetrics()
				rm := rms.AppendEmpty()
				ilms := rm.ScopeMetrics()
				ilm := ilms.AppendEmpty()
				ilm.Scope().SetName("github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver/memory")
				metricsArray := ilm.Metrics()
				met := metricsArray.AppendEmpty()
				met.SetName(histogramMetricName)
				met.SetEmptyHistogram()
				met.Histogram().SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
				hdps := met.Histogram().DataPoints()
				hdp := hdps.AppendEmpty()
				hdp.BucketCounts().FromRaw([]uint64{100})
				hdp.SetCount(100)
				hdp.SetSum(0)
				return md
			},
			msrc: metrics.MetricSourceOpenTelemetryCollectorHostmetricsReceiver,
		},
		{
			name: "metric origin in timeseries",
			genMetrics: func(_ *testing.T) pmetric.Metrics {
				md := pmetric.NewMetrics()
				rms := md.ResourceMetrics()
				rm := rms.AppendEmpty()
				ilms := rm.ScopeMetrics()
				ilm := ilms.AppendEmpty()
				ilm.Scope().SetName("github.com/open-telemetry/opentelemetry-collector-contrib/receiver/kubeletstatsreceiver")
				metricsArray := ilm.Metrics()
				met := metricsArray.AppendEmpty()
				met.SetName(numberMetricName)
				met.SetEmptyGauge()
				gdps := met.Gauge().DataPoints()
				gdp := gdps.AppendEmpty()
				gdp.SetIntValue(100)
				return md
			},
			msrc: metrics.MetricSourceOpenTelemetryCollectorKubeletstatsReceiver,
		},
		{
			name: "unknown metric origin",
			genMetrics: func(_ *testing.T) pmetric.Metrics {
				md := pmetric.NewMetrics()
				rms := md.ResourceMetrics()
				rm := rms.AppendEmpty()
				ilms := rm.ScopeMetrics()
				ilm := ilms.AppendEmpty()
				ilm.Scope().SetName("github.com/open-telemetry/opentelemetry-collector-contrib/receiver/myreceiver")
				metricsArray := ilm.Metrics()
				met := metricsArray.AppendEmpty()
				met.SetName(numberMetricName)
				met.SetEmptyGauge()
				gdps := met.Gauge().DataPoints()
				gdp := gdps.AppendEmpty()
				gdp.SetIntValue(100)
				return md
			},
			msrc: metrics.MetricSourceOpenTelemetryCollectorUnknown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &metricRecorder{}
			ctx := context.Background()
			f := NewFactoryForOTelAgent(rec, &MockTagEnricher{}, func(context.Context) (string, error) {
				return "", nil
			}, nil, otel.NewDisabledGatewayUsage(), TelemetryStore{})
			cfg := f.CreateDefaultConfig().(*ExporterConfig)
			exp, err := f.CreateMetrics(
				ctx,
				exportertest.NewNopSettings(component.MustNewType("datadog")),
				cfg,
			)
			require.NoError(t, err)
			require.NoError(t, exp.Start(ctx, componenttest.NewNopHost()))
			require.NoError(t, exp.ConsumeMetrics(ctx, tt.genMetrics(t)))
			require.NoError(t, exp.Shutdown(ctx))

			for _, serie := range rec.series {
				if serie.Name != numberMetricName {
					continue
				}
				assert.Equal(t, serie.Source, tt.msrc)
			}
			for _, sketch := range rec.sketchSeriesList {
				if sketch.Name != histogramMetricName {
					continue
				}
				assert.Equal(t, sketch.Source, tt.msrc)
			}
		})
	}
}

func TestMetricPrefix(t *testing.T) {
	testMetricPrefixWithFeatureGates(t, false, "datadog_trace_agent_retries", "otelcol_datadog_trace_agent_retries")
	testMetricPrefixWithFeatureGates(t, false, "system.memory.usage", "otel.system.memory.usage")
	testMetricPrefixWithFeatureGates(t, false, "process.cpu.utilization", "otel.process.cpu.utilization")
	testMetricPrefixWithFeatureGates(t, false, "kafka.producer.request-rate", "otel.kafka.producer.request-rate")

	testMetricPrefixWithFeatureGates(t, true, "datadog_trace_agent_retries", "datadog_trace_agent_retries")
	testMetricPrefixWithFeatureGates(t, true, "system.memory.usage", "system.memory.usage")
	testMetricPrefixWithFeatureGates(t, true, "process.cpu.utilization", "process.cpu.utilization")
	testMetricPrefixWithFeatureGates(t, true, "kafka.producer.request-rate", "kafka.producer.request-rate")
}

func testMetricPrefixWithFeatureGates(t *testing.T, disablePrefix bool, inName string, outName string) {
	prevVal := pkgdatadog.MetricRemappingDisabledFeatureGate.IsEnabled()
	require.NoError(t, featuregate.GlobalRegistry().Set(pkgdatadog.MetricRemappingDisabledFeatureGate.ID(), disablePrefix))
	defer func() {
		require.NoError(t, featuregate.GlobalRegistry().Set(pkgdatadog.MetricRemappingDisabledFeatureGate.ID(), prevVal))
	}()

	rec := &metricRecorder{}
	ctx := context.Background()
	f := NewFactoryForOTelAgent(rec, &MockTagEnricher{}, func(context.Context) (string, error) {
		return "", nil
	}, nil, otel.NewDisabledGatewayUsage(), TelemetryStore{})
	cfg := f.CreateDefaultConfig().(*ExporterConfig)
	exp, err := f.CreateMetrics(
		ctx,
		exportertest.NewNopSettings(component.MustNewType("datadog")),
		cfg,
	)
	require.NoError(t, err)
	require.NoError(t, exp.Start(ctx, componenttest.NewNopHost()))

	md := pmetric.NewMetrics()
	rms := md.ResourceMetrics()
	rm := rms.AppendEmpty()
	ilms := rm.ScopeMetrics()
	ilm := ilms.AppendEmpty()
	metricsArray := ilm.Metrics()
	met := metricsArray.AppendEmpty()
	met.SetName(inName)
	met.SetEmptySum()
	met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	dp := met.Sum().DataPoints().AppendEmpty()
	dp.SetIntValue(100)

	require.NoError(t, exp.ConsumeMetrics(ctx, md))
	require.NoError(t, exp.Shutdown(ctx))

	for _, serie := range rec.series {
		if serie.Name == outName {
			return
		}
	}
	t.Errorf("%s not found in metrics", outName)
}

func newMetrics(
	histogramMetricName string,
	histogramDataPoint pmetric.HistogramDataPoint,
	numberMetricName string,
	numberDataPoint pmetric.NumberDataPoint,
) pmetric.Metrics {
	md := pmetric.NewMetrics()
	rms := md.ResourceMetrics()
	rm := rms.AppendEmpty()
	ilms := rm.ScopeMetrics()
	ilm := ilms.AppendEmpty()
	metricsArray := ilm.Metrics()
	metricsArray.AppendEmpty() // first one is TypeNone to test that it's ignored

	// Histgram
	met := metricsArray.AppendEmpty()
	met.SetName(histogramMetricName)
	met.SetEmptyHistogram()
	met.Histogram().SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	hdps := met.Histogram().DataPoints()
	hdp := hdps.AppendEmpty()
	hdp.SetCount(histogramDataPoint.Count())
	hdp.SetSum(histogramDataPoint.Sum())
	histogramDataPoint.BucketCounts().CopyTo(hdp.BucketCounts())
	histogramDataPoint.ExplicitBounds().CopyTo(hdp.ExplicitBounds())
	hdp.SetTimestamp(histogramDataPoint.Timestamp())
	hdpAttrs := hdp.Attributes()
	histogramDataPoint.Attributes().CopyTo(hdpAttrs)

	// Gauge
	met = metricsArray.AppendEmpty()
	met.SetName(numberMetricName)
	met.SetEmptyGauge()
	gdps := met.Gauge().DataPoints()
	gdp := gdps.AppendEmpty()
	gdp.SetTimestamp(numberDataPoint.Timestamp())
	gdp.SetIntValue(numberDataPoint.IntValue())
	gdpAttrs := gdp.Attributes()
	numberDataPoint.Attributes().CopyTo(gdpAttrs)

	return md
}

func TestUsageMetric_AgentOTLPIngest(t *testing.T) {
	rec := &metricRecorder{}
	ctx := context.Background()
	telemetryComp := fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule())
	store := TelemetryStore{
		OTLPIngestMetrics: telemetryComp.NewGauge(
			"runtime",
			"datadog_agent_otlp_ingest_metrics",
			[]string{"version", "command", "host"},
			"Usage metric of OTLP metrics in OTLP ingestion",
		),
	}
	f := NewFactoryForAgent(rec, &MockTagEnricher{}, func(context.Context) (string, error) {
		return "agent-host", nil
	}, store)
	cfg := f.CreateDefaultConfig().(*ExporterConfig)
	exp, err := f.CreateMetrics(
		ctx,
		exportertest.NewNopSettings(component.MustNewType("serializer")),
		cfg,
	)
	require.NoError(t, err)
	require.NoError(t, exp.Start(ctx, componenttest.NewNopHost()))

	h := pmetric.NewHistogramDataPoint()
	h.BucketCounts().FromRaw([]uint64{100})
	h.SetCount(100)
	h.SetSum(0)
	n := pmetric.NewNumberDataPoint()
	n.SetIntValue(777)
	md := newMetrics("test-histogram", h, "test-gauge", n)
	require.NoError(t, exp.ConsumeMetrics(ctx, md))
	require.NoError(t, exp.Shutdown(ctx))

	usageMetric, err := telemetryComp.GetGaugeMetric("runtime", "datadog_agent_otlp_ingest_metrics")
	require.NoError(t, err)
	require.Len(t, usageMetric, 1)
	assert.Equal(t, map[string]string{"host": "agent-host", "command": "otelcol", "version": "latest"}, usageMetric[0].Tags())
	assert.Equal(t, 1.0, usageMetric[0].Value())

	_, err = telemetryComp.GetGaugeMetric("runtime", "datadog_agent_ddot_metrics")
	assert.ErrorContains(t, err, "runtime__datadog_agent_ddot_metrics not found")
}

func TestUsageMetric_DDOT(t *testing.T) {
	rec := &metricRecorder{}
	ctx := context.Background()
	telemetryComp := fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule())
	store := TelemetryStore{
		DDOTMetrics: telemetryComp.NewGauge(
			"runtime",
			"datadog_agent_ddot_metrics",
			[]string{"version", "command", "host", "task_arn"},
			"Usage metric of OTLP metrics in OTLP ingestion",
		),
	}
	f := NewFactoryForOTelAgent(rec, &MockTagEnricher{}, func(context.Context) (string, error) {
		return "agent-host", nil
	}, nil, otel.NewDisabledGatewayUsage(), store)
	cfg := f.CreateDefaultConfig().(*ExporterConfig)
	exp, err := f.CreateMetrics(
		ctx,
		exportertest.NewNopSettings(component.MustNewType("datadog")),
		cfg,
	)
	require.NoError(t, err)
	require.NoError(t, exp.Start(ctx, componenttest.NewNopHost()))

	md := pmetric.NewMetrics()
	rms := md.ResourceMetrics()
	rm := rms.AppendEmpty()
	rm.Resource().Attributes().PutStr("datadog.host.name", "test-host")
	ilms := rm.ScopeMetrics()
	ilm := ilms.AppendEmpty()
	metricsArray := ilm.Metrics()
	met := metricsArray.AppendEmpty()
	met.SetName("test-metric")
	met.SetEmptySum()
	met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	dp := met.Sum().DataPoints().AppendEmpty()
	dp.SetIntValue(100)

	require.NoError(t, exp.ConsumeMetrics(ctx, md))
	require.NoError(t, exp.Shutdown(ctx))

	usageMetric, err := telemetryComp.GetGaugeMetric("runtime", "datadog_agent_ddot_metrics")
	require.NoError(t, err)
	require.Len(t, usageMetric, 1)
	assert.Equal(t, map[string]string{"host": "test-host", "command": "otelcol", "version": "latest", "task_arn": ""}, usageMetric[0].Tags())
	assert.Equal(t, 1.0, usageMetric[0].Value())

	_, err = telemetryComp.GetGaugeMetric("runtime", "datadog_agent_otlp_ingest_metrics")
	assert.ErrorContains(t, err, "runtime__datadog_agent_otlp_ingest_metrics not found")
}
