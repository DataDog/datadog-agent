// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build test

package serializerexporter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/featuregates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/featuregate"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	delegatedauthnoopfx "github.com/DataDog/datadog-agent/comp/core/delegatedauth/fx-noop"
	logdef "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretnooptypes "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl/types"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	mocktelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	defaultforwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	defaultforwarderimpl "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/impl"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	metricscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-otel"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	source "github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
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
		r.sketchSeriesList = append(r.sketchSeriesList, c.(*metrics.SketchSeries))
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
			wantSketchTags: tagset.NewCompositeTags([]string{}, nil),
			wantSerieTags:  tagset.NewCompositeTags([]string{}, nil),
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
				[]string{
					"serverless_tag1:test1", "serverless_tag2:test2", "serverless_tag3:test3",
					"histogram_1_id:value1", "histogram_2_id:value2", "histogram_3_id:value3",
				},
				nil,
			),
			wantSerieTags: tagset.NewCompositeTags(
				[]string{
					"serverless_tag1:test1", "serverless_tag2:test2", "serverless_tag3:test3",
					"gauge_1_id:value1", "gauge_2_id:value2", "gauge_3_id:value3",
				},
				nil,
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
			wantSketchTags: tagset.NewCompositeTags([]string{}, nil),
			wantSerieTags:  tagset.NewCompositeTags([]string{}, nil),
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
				[]string{
					"serverless_tag1:test1", "serverless_tag2:test2", "serverless_tag3:test3",
					"histogram_1_id:value1", "histogram_2_id:value2", "histogram_3_id:value3",
				},
				nil,
			),
			wantSerieTags: tagset.NewCompositeTags(
				[]string{
					"serverless_tag1:test1", "serverless_tag2:test2", "serverless_tag3:test3",
					"gauge_1_id:value1", "gauge_2_id:value2", "gauge_3_id:value3",
				},
				nil,
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
			wantSketchTags: tagset.NewCompositeTags([]string{
				"instrumentation_scope:my_library", "instrumentation_scope_version:v1.0.0",
			}, nil),
			wantSerieTags: tagset.NewCompositeTags([]string{
				"instrumentation_scope:my_library", "instrumentation_scope_version:v1.0.0",
			}, nil),
			instrumentationScopeMetadataAsTags: true,
		},
		{
			name: "service.instance.id resource attribute becomes tag",
			genMetrics: func(_ *testing.T) pmetric.Metrics {
				h := pmetric.NewHistogramDataPoint()
				h.BucketCounts().FromRaw([]uint64{100})
				h.SetCount(100)
				h.SetSum(0)

				n := pmetric.NewNumberDataPoint()
				n.SetIntValue(777)
				md := newMetrics(histogramMetricName, h, numberMetricName, n)
				md.ResourceMetrics().At(0).Resource().Attributes().PutStr("service.instance.id", "my-instance-123")
				return md
			},
			extraTags: []string{},
			wantSketchTags: tagset.NewCompositeTags([]string{
				"service.instance.id:my-instance-123",
			}, nil),
			wantSerieTags: tagset.NewCompositeTags([]string{
				"service.instance.id:my-instance-123",
			}, nil),
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
				assert.Equal(t, tagset.NewCompositeTags([]string{}, nil), rec.sketchSeriesList[0].Tags)
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
						assert.Equal(t, tagset.NewCompositeTags([]string{}, nil), s.Tags)
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
	prevNew := featuregates.DisableMetricRemappingFeatureGate.IsEnabled()
	require.NoError(t, featuregate.GlobalRegistry().Set(featuregates.DisableMetricRemappingFeatureGate.ID(), disablePrefix))
	defer func() {
		require.NoError(t, featuregate.GlobalRegistry().Set(featuregates.DisableMetricRemappingFeatureGate.ID(), prevNew))
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

func TestRunningMetricForPayloadContents(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(m pmetric.Metric)
		wantRunning bool
	}{
		{
			name: "apm stats only",
			setup: func(m pmetric.Metric) {
				m.SetName("dd.internal.stats.payload")
				m.SetEmptySum()
			},
			wantRunning: false,
		},
		{
			name: "real metric",
			setup: func(m pmetric.Metric) {
				m.SetName("my.metric")
				m.SetEmptyGauge().DataPoints().AppendEmpty().SetDoubleValue(1)
			},
			wantRunning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newDefaultConfig().(*ExporterConfig)

			set := exportertest.NewNopSettings(component.MustNewType("datadog"))
			attributesTranslator, err := attributes.NewTranslator(set.TelemetrySettings)
			require.NoError(t, err)
			hostGetter := SourceProviderFunc(func(context.Context) (string, error) { return "test-hostname", nil })
			tr, err := translatorFromConfig(set.TelemetrySettings, attributesTranslator, cfg.Metrics.Metrics, hostGetter, nil)
			require.NoError(t, err)

			createConsumer := func([]string, string, component.BuildInfo) SerializerConsumer {
				return &collectorConsumer{
					serializerConsumer: &serializerConsumer{},
					seenHosts:          make(map[string]struct{}),
					seenTagSets:        make(map[tagSetKey][]string),
					getPushTime:        func() uint64 { return 0 },
				}
			}

			rec := &metricRecorder{}
			exp, err := NewExporter(rec, cfg, hostGetter, createConsumer, tr, set, nil, otel.NewDisabledGatewayUsage(), nil, nil, ossCollector)
			require.NoError(t, err)

			md := pmetric.NewMetrics()
			m := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
			tt.setup(m)

			require.NoError(t, exp.ConsumeMetrics(t.Context(), md))

			var names []string
			for _, serie := range rec.series {
				names = append(names, serie.Name)
			}
			if tt.wantRunning {
				assert.Contains(t, names, "otel.datadog_exporter.metrics.running")
			} else {
				assert.NotContains(t, names, "otel.datadog_exporter.metrics.running")
				assert.NotContains(t, names, "otel.datadog_exporter.metrics.running.fargate")
			}
		})
	}
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
	telemetryComp := fxutil.Test[telemetry.Mock](t, mocktelemetry.Module())
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
	telemetryComp := fxutil.Test[telemetry.Mock](t, mocktelemetry.Module())
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
	telemetryComp := fxutil.Test[telemetry.Mock](t, mocktelemetry.Module())
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
		isEnabledGate   bool
		expectedMetrics []string
	}{
		{
			isEnabledGate: false,
			expectedMetrics: []string{
				"otel.system.filesystem.utilization",
				"otel.process.runtime.go.goroutines",
				"otel.process.runtime.dotnet.exceptions.count",
				"otel.process.runtime.jvm.threads.count",
				"runtime.go.num_goroutine",
				"runtime.dotnet.exceptions.count",
				"jvm.thread_count",
				"datadog.agent.otlp.metrics",
				"datadog.agent.otlp.runtime_metrics",
				"datadog.agent.otlp.runtime_metrics",
				"datadog.agent.otlp.runtime_metrics",
				"datadog.otel.gateway.configured",
			},
		},
		{
			isEnabledGate: true,
			expectedMetrics: []string{
				"system.filesystem.utilization",
				"process.runtime.go.goroutines",
				"process.runtime.dotnet.exceptions.count",
				"process.runtime.jvm.threads.count",
				"datadog.agent.otlp.metrics",
				"datadog.otel.gateway.configured",
			},
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("isEnabledGate=%v", tt.isEnabledGate), func(t *testing.T) {
			reg := featuregate.GlobalRegistry()
			prevNewVal := featuregates.DisableMetricRemappingFeatureGate.IsEnabled()
			require.NoError(t, reg.Set(featuregates.DisableMetricRemappingFeatureGate.ID(), tt.isEnabledGate))
			defer func() {
				require.NoError(t, reg.Set(featuregates.DisableMetricRemappingFeatureGate.ID(), prevNewVal))
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

func TestDeltaSumAsRateAttribute(t *testing.T) {
	tests := []struct {
		name          string
		genMetrics    func() pmetric.Metrics
		wantType      metrics.APIMetricType
		wantName      string
		checkHasAsTag bool
	}{
		{
			name: "delta sum with as_type=rate becomes rate",
			genMetrics: func() pmetric.Metrics {
				md := pmetric.NewMetrics()
				rm := md.ResourceMetrics().AppendEmpty()
				ilm := rm.ScopeMetrics().AppendEmpty()
				met := ilm.Metrics().AppendEmpty()
				met.SetName("test.delta.sum")
				met.SetEmptySum()
				met.Sum().SetIsMonotonic(false)
				met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
				dp := met.Sum().DataPoints().AppendEmpty()
				dp.SetIntValue(100)
				dp.Attributes().PutStr("datadog.metric.as_type", "rate")
				dp.Attributes().PutStr("env", "test")
				return md
			},
			wantType:      metrics.APIRateType,
			wantName:      "test.delta.sum",
			checkHasAsTag: true,
		},
		{
			name: "delta sum without attribute stays count",
			genMetrics: func() pmetric.Metrics {
				md := pmetric.NewMetrics()
				rm := md.ResourceMetrics().AppendEmpty()
				ilm := rm.ScopeMetrics().AppendEmpty()
				met := ilm.Metrics().AppendEmpty()
				met.SetName("test.delta.sum")
				met.SetEmptySum()
				met.Sum().SetIsMonotonic(false)
				met.Sum().SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
				dp := met.Sum().DataPoints().AppendEmpty()
				dp.SetIntValue(100)
				return md
			},
			wantType: metrics.APICountType,
			wantName: "test.delta.sum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			require.NoError(t, exp.ConsumeMetrics(t.Context(), tt.genMetrics()))
			require.NoError(t, exp.Shutdown(t.Context()))

			found := false
			for _, s := range rec.series {
				if s.Name == tt.wantName {
					found = true
					assert.Equal(t, tt.wantType, s.MType, "metric type mismatch for %s", s.Name)
					if tt.checkHasAsTag {
						hasAsTag := false
						s.Tags.ForEach(func(tag string) {
							if tag == "datadog.metric.as_type:rate" {
								hasAsTag = true
							}
						})
						assert.True(t, hasAsTag,
							"control attribute should be present as a tag for debugging")
					}
					break
				}
			}
			assert.True(t, found, "metric %s not found in recorded series", tt.wantName)
		})
	}
}

// TestSyncForwarder_PropagatesErrors is the headline test for OTAGENT-1024:
// when the sync forwarder is on, a 5xx response from intake must surface back
// through ConsumeMetrics rather than be silently swallowed.
// Simulates DDOT: OTelSyncForwarder is injected via initSyncSerializerForTest,
// mirroring cmd/otel-agent/subcommands/run/command.go.
func TestSyncForwarder_PropagatesErrors(t *testing.T) {
	restore := setSyncForwarderGate(t, true)
	defer restore()

	intake := newFakeIntake(http.StatusInternalServerError)
	defer intake.Close()

	cfg := benchExporterConfig(t, intake.URL)
	exp := buildBenchExporter(t, cfg)
	defer func() { _ = exp.Shutdown(context.Background()) }()

	mc, ok := exp.(metricsConsumer)
	require.True(t, ok)

	err := mc.ConsumeMetrics(context.Background(), makeGaugeMetrics(50))
	require.Error(t, err, "5xx from intake must surface back through ConsumeMetrics")
	require.GreaterOrEqual(t, intake.requests.Load(), int64(1), "intake should have received at least one request")
}

// TestSyncForwarder_PermanentError verifies that a non-retryable intake
// response (400/403/413) is wrapped in consumererror.NewPermanent so the
// exporterhelper queue does not retry it. Exercises the allSendsPermanent
// true-branch in ConsumeMetrics, which TestSyncForwarder_PropagatesErrors
// (5xx, transient) never reaches.
func TestSyncForwarder_PermanentError(t *testing.T) {
	restore := setSyncForwarderGate(t, true)
	defer restore()

	intake := newFakeIntake(http.StatusBadRequest)
	defer intake.Close()

	cfg := benchExporterConfig(t, intake.URL)
	exp := buildBenchExporter(t, cfg)
	defer func() { _ = exp.Shutdown(context.Background()) }()

	mc, ok := exp.(metricsConsumer)
	require.True(t, ok)

	err := mc.ConsumeMetrics(context.Background(), makeGaugeMetrics(50))
	require.Error(t, err, "400 from intake must surface back through ConsumeMetrics")
	require.True(t, consumererror.IsPermanent(err), "400 is non-retryable and must be wrapped as a permanent error")
	requestsAfterFirstAttempt := intake.requests.Load()

	err = mc.ConsumeMetrics(context.Background(), makeGaugeMetrics(50))
	require.Error(t, err, "400 from intake must surface back through ConsumeMetrics")
	require.True(t, consumererror.IsPermanent(err))
	require.Equal(t, requestsAfterFirstAttempt*2, intake.requests.Load(),
		"a second ConsumeMetrics call should issue the same number of requests as the first, with no exporterhelper retries in between")
}

// TestSyncForwarder_RetryOnTransientError verifies that the OTel
// exporterhelper retry layer retries on transient 5xx responses and that
// ConsumeMetrics ultimately returns nil once the intake starts succeeding.
func TestSyncForwarder_RetryOnTransientError(t *testing.T) {
	restore := setSyncForwarderGate(t, true)
	defer restore()

	const failFirst = 2
	intake := newFakeIntakeWithHandler(func(n int64, w http.ResponseWriter, _ *http.Request) {
		if n <= failFirst {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	})
	defer intake.Close()

	cfg := retryExporterConfig(t, intake.URL)
	exp := buildBenchExporter(t, cfg)
	defer func() { _ = exp.Shutdown(context.Background()) }()

	mc, ok := exp.(metricsConsumer)
	require.True(t, ok)

	err := mc.ConsumeMetrics(context.Background(), makeGaugeMetrics(10))
	require.NoError(t, err, "ConsumeMetrics should succeed after retries")
	require.GreaterOrEqual(t, intake.requests.Load(), int64(failFirst+1),
		"intake should have received at least %d requests (failures + success)", failFirst+1)
}

// TestSyncForwarder_RetryBudgetExhausted verifies that when the sync
// forwarder is on and the intake consistently fails, the error surfaces
// through ConsumeMetrics once the retry budget is exhausted.
func TestSyncForwarder_RetryBudgetExhausted(t *testing.T) {
	restore := setSyncForwarderGate(t, true)
	defer restore()

	intake := newFakeIntake(http.StatusInternalServerError)
	defer intake.Close()

	cfg := retryExporterConfig(t, intake.URL)
	cfg.RetryConfig.MaxElapsedTime = 200 * time.Millisecond
	exp := buildBenchExporter(t, cfg)
	defer func() { _ = exp.Shutdown(context.Background()) }()

	mc, ok := exp.(metricsConsumer)
	require.True(t, ok)

	err := mc.ConsumeMetrics(context.Background(), makeGaugeMetrics(10))
	require.Error(t, err, "ConsumeMetrics should return error after retry budget is exhausted")
	require.GreaterOrEqual(t, intake.requests.Load(), int64(1),
		"intake should have received at least one request before budget was exhausted")
}

// TestSyncForwarder_RetryConfig_Respected verifies that a RetryConfig with a
// short MaxElapsedTime is honoured: the exporter gives up faster than one with
// a longer budget.
func TestSyncForwarder_RetryConfig_Respected(t *testing.T) {
	restore := setSyncForwarderGate(t, true)
	defer restore()

	intake := newFakeIntake(http.StatusInternalServerError)
	defer intake.Close()

	cfgShort := retryExporterConfig(t, intake.URL)
	cfgShort.RetryConfig.MaxElapsedTime = 50 * time.Millisecond

	cfgLong := retryExporterConfig(t, intake.URL)
	cfgLong.RetryConfig.MaxElapsedTime = 300 * time.Millisecond

	expShort := buildBenchExporter(t, cfgShort)
	defer func() { _ = expShort.Shutdown(context.Background()) }()

	mcShort, ok := expShort.(metricsConsumer)
	require.True(t, ok)
	require.Error(t, mcShort.ConsumeMetrics(context.Background(), makeGaugeMetrics(10)))
	reqsAfterShort := intake.requests.Load()

	expLong := buildBenchExporter(t, cfgLong)
	defer func() { _ = expLong.Shutdown(context.Background()) }()

	mcLong, ok := expLong.(metricsConsumer)
	require.True(t, ok)
	require.Error(t, mcLong.ConsumeMetrics(context.Background(), makeGaugeMetrics(10)))
	reqsAfterLong := intake.requests.Load()

	require.Greater(t, reqsAfterLong, reqsAfterShort,
		"longer retry budget should produce more intake requests (short=%d, long=%d)",
		reqsAfterShort, reqsAfterLong-reqsAfterShort)
}

// TestDefaultForwarder_SwallowsErrors documents the legacy behavior the
// feature gate exists to fix: with the gate off, intake 5xx is hidden from
// ConsumeMetrics. If this test ever starts failing, the legacy path has
// converged with the sync path and the feature gate can be retired.
func TestDefaultForwarder_SwallowsErrors(t *testing.T) {
	restore := setSyncForwarderGate(t, false)
	defer restore()

	intake := newFakeIntake(http.StatusInternalServerError)
	defer intake.Close()

	cfg := benchExporterConfig(t, intake.URL)
	exp := buildBenchExporter(t, cfg)
	defer func() { _ = exp.Shutdown(context.Background()) }()

	mc, ok := exp.(metricsConsumer)
	require.True(t, ok)

	// Default async forwarder enqueues the payload and returns nil immediately.
	require.NoError(t, mc.ConsumeMetrics(context.Background(), makeGaugeMetrics(50)))
}

// TestOSSSyncForwarder_PropagatesErrors verifies that the OSS Datadog exporter
// path (NewFactoryForOSSExporter, f.s == nil) also uses OTelSyncForwarder when
// the UseSyncForwarder gate is on, and propagates intake errors back to the caller.
func TestOSSSyncForwarder_PropagatesErrors(t *testing.T) {
	restore := setSyncForwarderGate(t, true)
	defer restore()

	intake := newFakeIntake(http.StatusInternalServerError)
	defer intake.Close()

	cfg := benchExporterConfig(t, intake.URL)
	f := NewFactoryForOSSExporter(component.MustNewType("datadog"), nil)
	exp, err := f.CreateMetrics(
		context.Background(),
		exportertest.NewNopSettings(component.MustNewType("datadog")),
		cfg,
	)
	require.NoError(t, err)
	require.NoError(t, exp.Start(context.Background(), componenttest.NewNopHost()))
	defer func() { _ = exp.Shutdown(context.Background()) }()

	mc, ok := exp.(metricsConsumer)
	require.True(t, ok)

	err = mc.ConsumeMetrics(context.Background(), makeGaugeMetrics(50))
	require.Error(t, err, "5xx from intake must surface back through ConsumeMetrics on the OSS exporter path")
	require.GreaterOrEqual(t, intake.requests.Load(), int64(1), "intake should have received at least one request")
}

// initSyncSerializerForTest creates a serializer backed by OTelSyncForwarder
// via a mini-Fx app. This simulates the DDOT production path where
// cmd/otel-agent/subcommands/run/command.go injects OTelSyncForwarder into the
// shared serializer (OTAGENT-1024). Not for use outside of tests.
func initSyncSerializerForTest(t testing.TB, logger *zap.Logger, cfg *ExporterConfig, sourceProvider source.Provider, httpClient *http.Client) (*serializer.Serializer, *defaultforwarderimpl.OTelSyncForwarder, error) {
	var f defaultforwarder.Forwarder
	var s *serializer.Serializer

	opts := []fx.Option{
		fx.WithLogger(func(log *zap.Logger) fxevent.Logger {
			return &fxevent.ZapLogger{Logger: log}
		}),
		fx.Supply(logger),
		fxutil.FxAgentBase(),
		fx.Provide(func() coreconfig.Component {
			pkgconfig := configmock.New(t)
			pkgconfig.Set("api_key", string(cfg.API.Key), pkgconfigmodel.SourceFile)
			pkgconfig.Set("site", cfg.API.Site, pkgconfigmodel.SourceFile)
			if cfg.Metrics.Metrics.TCPAddrConfig.Endpoint != "" {
				pkgconfig.Set("dd_url", cfg.Metrics.Metrics.TCPAddrConfig.Endpoint, pkgconfigmodel.SourceFile)
			}
			setupSerializer(pkgconfig, cfg)
			setupForwarder(pkgconfig)
			pkgconfig.Set("skip_ssl_validation", cfg.ClientConfig.InsecureSkipVerify, pkgconfigmodel.SourceFile)
			pkgconfig.Set("logging_frequency", int64(0), pkgconfigmodel.SourceAgentRuntime)
			return pkgconfig
		}),
		fx.Provide(func(log *zap.Logger) (logdef.Component, error) {
			zp := &datadog.Zaplogger{Logger: log}
			return zp, nil
		}),
		fx.Provide(func() string {
			s, err := sourceProvider.Source(context.TODO())
			if err != nil {
				return ""
			}
			return s.Identifier.Primary
		}),
		fx.Provide(newOrchestratorinterfaceimpl),
		fx.Provide(serializer.NewSerializer),
		metricscompressionfx.Module(),
		fx.Provide(func(c metricscompression.Component) compression.Compressor {
			return c
		}),
		fx.Provide(func() secrets.Component { return &secretnooptypes.SecretNoop{} }),
		delegatedauthnoopfx.Module(),
		fx.Populate(&f),
		fx.Populate(&s),
		fx.Provide(func(c coreconfig.Component, l logdef.Component, sec secrets.Component) (defaultforwarder.Forwarder, error) {
			eds, err := configutils.GetMultipleEndpoints(c)
			if err != nil {
				return nil, err
			}
			return defaultforwarderimpl.NewOTelSyncForwarder(c, l, sec, eds, httpClient)
		}),
	}

	app := fx.New(opts...)
	if err := app.Err(); err != nil {
		return nil, nil, err
	}

	sf, ok := f.(*defaultforwarderimpl.OTelSyncForwarder)
	if !ok {
		return nil, nil, errors.New("failed to cast forwarder to *defaultforwarderimpl.OTelSyncForwarder")
	}
	return s, sf, nil
}
