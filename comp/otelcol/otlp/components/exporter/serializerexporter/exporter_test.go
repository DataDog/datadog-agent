// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build test

package serializerexporter

import (
	"context"
	"fmt"
	"strings"
	"testing"

	pkgdatadog "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/featuregates"
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
			wantSketchTags: tagset.NewCompositeTags(nil, []string{}),
			wantSerieTags:  tagset.NewCompositeTags(nil, []string{}),
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
			wantSketchTags: tagset.NewCompositeTags(
				[]string{"serverless_tag1:test1", "serverless_tag2:test2", "serverless_tag3:test3"},
				[]string{"histogram_1_id:value1", "histogram_2_id:value2", "histogram_3_id:value3"},
			),
			wantSerieTags: tagset.NewCompositeTags(
				[]string{"serverless_tag1:test1", "serverless_tag2:test2", "serverless_tag3:test3"},
				[]string{"gauge_1_id:value1", "gauge_2_id:value2", "gauge_3_id:value3"},
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
			wantSketchTags: tagset.NewCompositeTags(nil, []string{}),
			wantSerieTags:  tagset.NewCompositeTags(nil, []string{}),
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
			wantSketchTags: tagset.NewCompositeTags(
				[]string{"serverless_tag1:test1", "serverless_tag2:test2", "serverless_tag3:test3"},
				[]string{"histogram_1_id:value1", "histogram_2_id:value2", "histogram_3_id:value3"},
			),
			wantSerieTags: tagset.NewCompositeTags(
				[]string{"serverless_tag1:test1", "serverless_tag2:test2", "serverless_tag3:test3"},
				[]string{"gauge_1_id:value1", "gauge_2_id:value2", "gauge_3_id:value3"},
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
			wantSketchTags: tagset.NewCompositeTags(nil, []string{
				"instrumentation_scope:my_library", "instrumentation_scope_version:v1.0.0",
			}),
			wantSerieTags: tagset.NewCompositeTags(nil, []string{
				"instrumentation_scope:my_library", "instrumentation_scope_version:v1.0.0",
			}),
			instrumentationScopeMetadataAsTags: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &metricRecorder{}
			ctx := context.Background()
			f := NewFactoryForOTelAgent(rec, func(context.Context) (string, error) {
				return "", nil
			}, nil, otel.NewDisabledGatewayUsage(), TelemetryStore{}, nil)
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
				assert.Equal(t, tagset.NewCompositeTags(nil, []string{}), rec.sketchSeriesList[0].Tags)
			}
			assert.True(t, len(rec.series) > 0)
			for _, s := range rec.series {
				if s.Name == "datadog.agent.otlp.metrics" {
					assert.Equal(t, tagset.NewCompositeTags([]string{}, nil), s.Tags)
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
						assert.Equal(t, tagset.NewCompositeTags(nil, []string{}), s.Tags)
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
			f := NewFactoryForOTelAgent(rec, func(context.Context) (string, error) {
				return "", nil
			}, nil, otel.NewDisabledGatewayUsage(), TelemetryStore{}, nil)
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
	f := NewFactoryForOTelAgent(rec, func(context.Context) (string, error) {
		return "", nil
	}, nil, otel.NewDisabledGatewayUsage(), TelemetryStore{}, nil)
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
	f := NewFactoryForAgent(rec, func(context.Context) (string, error) {
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
			"Usage metric of OTLP metrics in DDOT",
		),
		DDOTGWUsage: telemetryComp.NewGauge(
			"runtime",
			"datadog_agent_ddot_gateway_usage",
			[]string{"version", "command"},
			"Usage metric for GW deployments with DDOT",
		),
	}

	f := NewFactoryForOTelAgent(rec, func(context.Context) (string, error) {
		return "agent-host", nil
	}, nil, otel.NewDisabledGatewayUsage(), store, nil)
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

	// DDOT usage metric should be 1
	usageMetric, err := telemetryComp.GetGaugeMetric("runtime", "datadog_agent_ddot_metrics")
	require.NoError(t, err)
	require.Len(t, usageMetric, 1)
	assert.Equal(t, map[string]string{"host": "test-host", "command": "otelcol", "version": "latest", "task_arn": ""}, usageMetric[0].Tags())
	assert.Equal(t, 1.0, usageMetric[0].Value())

	// GW usage metric should be zero
	usageMetric, err = telemetryComp.GetGaugeMetric("runtime", "datadog_agent_ddot_gateway_usage")
	require.NoError(t, err)
	require.Len(t, usageMetric, 1)
	assert.Equal(t, map[string]string{"command": "otelcol", "version": "latest"}, usageMetric[0].Tags())
	assert.Equal(t, float64(0), usageMetric[0].Value())

	_, err = telemetryComp.GetGaugeMetric("runtime", "datadog_agent_otlp_ingest_metrics")
	assert.ErrorContains(t, err, "runtime__datadog_agent_otlp_ingest_metrics not found")
}

func usageMetricGW(t *testing.T, gwUsage otel.GatewayUsage, expGwUsage float64, expGwEnvVar float64) {
	rec := &metricRecorder{}
	ctx := context.Background()
	telemetryComp := fxutil.Test[telemetry.Mock](t, telemetryimpl.MockModule())
	store := TelemetryStore{
		DDOTGWUsage: telemetryComp.NewGauge(
			"runtime",
			"datadog_agent_ddot_gateway_usage",
			[]string{"version", "command"},
			"Usage metric for GW deployments with DDOT",
		),
	}

	DDOTGWEnvValue := telemetryComp.NewGauge(
		"runtime",
		"datadog_agent_ddot_gateway_configured",
		[]string{"version", "command"},
		"The value of DD_OTELCOLLECTOR_GATEWAY_MODE env. var set by Helm Chart or Operator",
	)

	if DDOTGWEnvValue != nil {
		DDOTGWEnvValue.Set(expGwEnvVar, "latest", "otelcol")
	}

	f := NewFactoryForOTelAgent(rec, func(context.Context) (string, error) {
		return "agent-host", nil
	}, nil, gwUsage, store, nil)

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

	usageMetric, err := telemetryComp.GetGaugeMetric("runtime", "datadog_agent_ddot_gateway_usage")
	require.NoError(t, err)
	require.Len(t, usageMetric, 1)
	assert.Equal(t, map[string]string{"command": "otelcol", "version": "latest"}, usageMetric[0].Tags())
	assert.Equal(t, expGwUsage, usageMetric[0].Value())

	usageGwEnvVar, err := telemetryComp.GetGaugeMetric("runtime", "datadog_agent_ddot_gateway_configured")
	require.NoError(t, err)
	require.Len(t, usageGwEnvVar, 1)
	assert.Equal(t, map[string]string{"command": "otelcol", "version": "latest"}, usageGwEnvVar[0].Tags())
	assert.Equal(t, expGwEnvVar, usageGwEnvVar[0].Value())

	_, err = telemetryComp.GetGaugeMetric("runtime", "datadog_agent_otlp_ingest_metrics")
	assert.ErrorContains(t, err, "runtime__datadog_agent_otlp_ingest_metrics not found")
}

func TestUsageMetric_GW(t *testing.T) {
	gwUsage := otel.NewGatewayUsage(false)
	// Force gw usage attribute to detect GW; two different host attributes will trigger that.
	attr := gwUsage.GetHostFromAttributesHandler()
	attr.OnHost("foo")
	attr.OnHost("bar")
	usageMetricGW(t, gwUsage, float64(1.0), float64(0.0))

	gwUsage = otel.NewGatewayUsage(false)
	usageMetricGW(t, gwUsage, float64(0.0), float64(0.0))

	gwUsage = otel.NewGatewayUsage(true)
	usageMetricGW(t, gwUsage, float64(1.0), float64(1.0))
}

func createTestMetricsWithRuntimeMetrics() pmetric.Metrics {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	ilm := rm.ScopeMetrics().AppendEmpty()
	metricsArray := ilm.Metrics()

	runtimeMetrics := []string{
		"system.filesystem.utilization",
		"process.runtime.go.goroutines",
		"process.runtime.dotnet.exceptions.count",
		"process.runtime.jvm.threads.count",
	}

	for _, metricName := range runtimeMetrics {
		met := metricsArray.AppendEmpty()
		met.SetName(metricName)
		dps := met.SetEmptyGauge().DataPoints()
		dp := dps.AppendEmpty()
		dp.SetTimestamp(0)
		dp.SetIntValue(42)
	}

	return md
}

func TestMetricRemapping(t *testing.T) {
	tests := []struct {
		newGate         bool
		oldGate         bool
		expectedMetrics []string
	}{
		{
			newGate: false,
			oldGate: false,
			expectedMetrics: []string{
				// Original metrics with otel. prefix
				"otel.system.filesystem.utilization",
				"otel.process.runtime.go.goroutines",
				"otel.process.runtime.dotnet.exceptions.count",
				"otel.process.runtime.jvm.threads.count",
				// Mapped runtime metrics
				"runtime.go.num_goroutine",
				"runtime.dotnet.exceptions.count",
				"jvm.thread_count",
				// Internal telemetry metrics
				"datadog.agent.otlp.metrics",
				"datadog.agent.otlp.runtime_metrics",
				"datadog.agent.otlp.runtime_metrics",
				"datadog.agent.otlp.runtime_metrics",
				"datadog.otel.gateway.configured",
			},
		},
		{
			newGate: true,
			oldGate: false,
			expectedMetrics: []string{
				// Original metrics without remapping
				"system.filesystem.utilization",
				"process.runtime.go.goroutines",
				"process.runtime.dotnet.exceptions.count",
				"process.runtime.jvm.threads.count",
				// Internal telemetry metrics
				"datadog.agent.otlp.metrics",
				"datadog.otel.gateway.configured",
			},
		},
		{
			newGate: false,
			oldGate: true,
			expectedMetrics: []string{
				// Original metrics without prefix
				"system.filesystem.utilization",
				"process.runtime.go.goroutines",
				"process.runtime.dotnet.exceptions.count",
				"process.runtime.jvm.threads.count",
				// Mapped runtime metrics
				"runtime.go.num_goroutine",
				"runtime.dotnet.exceptions.count",
				"jvm.thread_count",
				// Internal telemetry metrics
				"datadog.agent.otlp.metrics",
				"datadog.agent.otlp.runtime_metrics",
				"datadog.agent.otlp.runtime_metrics",
				"datadog.agent.otlp.runtime_metrics",
				"datadog.otel.gateway.configured",
			},
		},
		{
			newGate: true,
			oldGate: true,
			expectedMetrics: []string{
				// Original metrics without remapping (new gate takes precedence)
				"system.filesystem.utilization",
				"process.runtime.go.goroutines",
				"process.runtime.dotnet.exceptions.count",
				"process.runtime.jvm.threads.count",
				// Internal telemetry metrics
				"datadog.agent.otlp.metrics",
				"datadog.otel.gateway.configured",
			},
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("new=%v,old=%v", tt.newGate, tt.oldGate), func(t *testing.T) {
			reg := featuregate.GlobalRegistry()
			prevNewVal := featuregates.DisableMetricRemappingFeatureGate.IsEnabled()
			prevOldVal := featuregates.MetricRemappingDisabledFeatureGate.IsEnabled()
			require.NoError(t, reg.Set(featuregates.DisableMetricRemappingFeatureGate.ID(), tt.newGate))
			require.NoError(t, reg.Set(featuregates.MetricRemappingDisabledFeatureGate.ID(), tt.oldGate))
			defer func() {
				require.NoError(t, reg.Set(featuregates.DisableMetricRemappingFeatureGate.ID(), prevNewVal))
				require.NoError(t, reg.Set(featuregates.MetricRemappingDisabledFeatureGate.ID(), prevOldVal))
			}()

			rec := &metricRecorder{}
			f := NewFactoryForOTelAgent(rec, func(context.Context) (string, error) {
				return "", nil
			}, nil, otel.NewDisabledGatewayUsage(), TelemetryStore{}, nil)
			cfg := f.CreateDefaultConfig().(*ExporterConfig)
			exp, err := f.CreateMetrics(
				t.Context(),
				exportertest.NewNopSettings(component.MustNewType("datadog")),
				cfg,
			)
			require.NoError(t, err)
			require.NoError(t, exp.Start(t.Context(), componenttest.NewNopHost()))
			testMetrics := createTestMetricsWithRuntimeMetrics()
			err = exp.ConsumeMetrics(t.Context(), testMetrics)
			require.NoError(t, err)
			require.NoError(t, exp.Shutdown(t.Context()))

			actualMetrics := make([]string, 0, len(rec.series))
			for _, s := range rec.series {
				actualMetrics = append(actualMetrics, s.Name)
			}
			assert.ElementsMatch(t, tt.expectedMetrics, actualMetrics)
		})
	}
}
