// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"context"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics/internal/instrumentationlibrary"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics/internal/instrumentationscope"
)

// minimalTranslator is a lightweight translator for OTLP metrics that only supports
// delta temporality metrics and does not perform cumulative-to-delta conversion.
// Use this translator when metrics are already in delta format or when you want to
// forward metrics without stateful conversion.
//
// Unlike DefaultTranslator, minimalTranslator:
//   - Does not maintain a cache for cumulative-to-delta conversion
//   - Skips unsupported metrics (cumulative monotonic sums, cumulative histograms) instead of converting them
//   - Does not dual wrie metrics nor add prefix OTel
//
// Use NewMinimalTranslator to create instances.
type minimalTranslator struct {
	logger               *zap.Logger
	attributesTranslator *attributes.Translator
	cfg                  translatorConfig
	mapper               mapper
}

// NewMinimalTranslator creates a new minimal translator for OTLP metrics.
//
// The minimal translator is designed for use cases where:
//   - Metrics are already in delta temporality format
//   - Cumulative-to-delta conversion is not needed
//   - Lower memory footprint is desired
//
// Unsupported metrics (cumulative monotonic sums, cumulative histograms) are logged and skipped
// rather than returning an error.
func NewMinimalTranslator(logger *zap.Logger, attributesTranslator *attributes.Translator, options ...TranslatorOption) (Provider, error) {
	cfg := translatorConfig{
		HistMode:                             HistogramModeDistributions,
		SendHistogramAggregations:            false,
		Quantiles:                            false,
		NumberMode:                           NumberModeCumulativeToDelta,
		InitialCumulMonoValueMode:            InitialCumulMonoValueModeAuto,
		InstrumentationLibraryMetadataAsTags: false,
		sweepInterval:                        1800,
		deltaTTL:                             3600,
		fallbackSourceProvider:               &noSourceProvider{},
		originProduct:                        OriginProductUnknown,
	}

	for _, opt := range options {
		err := opt(&cfg)
		if err != nil {
			return nil, err
		}
	}

	return &minimalTranslator{
		logger:               logger,
		attributesTranslator: attributesTranslator,
		cfg:                  cfg,
		mapper:               newLossLessMapper(cfg, logger),
	}, nil
}

func (t *minimalTranslator) MapMetrics(ctx context.Context, md pmetric.Metrics, consumer Consumer, hostFromAttributesHandler attributes.HostFromAttributesHandler) (Metadata, error) {
	metadata := Metadata{
		Languages: []string{},
	}
	rms := md.ResourceMetrics()
	for i := 0; i < rms.Len(); i++ {
		rm := rms.At(i)
		src, err := resolveSource(ctx, t.attributesTranslator, rm.Resource(), t.cfg.fallbackSourceProvider, hostFromAttributesHandler)
		if err != nil {
			return metadata, err
		}

		var host string
		if src.Kind == source.HostnameKind {
			host = src.Identifier
			// Don't consume the host yet, first check if we have any nonAPM metrics.
		}

		// seenNonAPMMetrics is used to determine if we have seen any non-APM metrics in this ResourceMetrics.
		// If we have only seen APM metrics, we don't want to consume the host.
		var seenNonAPMMetrics bool

		// Fetch tags from attributes.
		attributeTags := attributes.TagsFromAttributes(rm.Resource().Attributes())
		ilms := rm.ScopeMetrics()
		rattrs := rm.Resource().Attributes()
		for j := 0; j < ilms.Len(); j++ {
			ilm := ilms.At(j)
			metricsArray := ilm.Metrics()

			var additionalTags []string
			if t.cfg.InstrumentationScopeMetadataAsTags {
				additionalTags = append(attributeTags, instrumentationscope.TagsFromInstrumentationScopeMetadata(ilm.Scope())...)
			} else if t.cfg.InstrumentationLibraryMetadataAsTags {
				additionalTags = append(attributeTags, instrumentationlibrary.TagsFromInstrumentationLibraryMetadata(ilm.Scope())...)
			} else {
				additionalTags = attributeTags
			}

			scopeName := ilm.Scope().Name()

			for k := 0; k < metricsArray.Len(); k++ {
				md := metricsArray.At(k)

				if md.Name() == keyStatsPayload && md.Type() == pmetric.MetricTypeSum {
					// these metrics are an APM Stats payload; consume it as such
					for l := 0; l < md.Sum().DataPoints().Len(); l++ {
						if payload, ok := md.Sum().DataPoints().At(l).Attributes().Get(keyStatsPayload); ok && t.cfg.statsOut != nil && payload.Type() == pcommon.ValueTypeBytes {
							t.cfg.statsOut <- payload.Bytes().AsRaw()
						}
					}
					continue
				}
				if _, ok := runtimeMetricsMappings[md.Name()]; ok {
					metadata.Languages = extractLanguageTag(md.Name(), metadata.Languages)
				} else {
					seenNonAPMMetrics = true
				}
				err := t.mapToDDFormat(ctx, md, consumer, additionalTags, host, scopeName, rattrs)
				if err != nil {
					return metadata, err
				}
			}

		}

		// Only consume the source if we have seen non-APM metrics.
		if seenNonAPMMetrics {
			switch src.Kind {
			case source.HostnameKind:
				if c, ok := consumer.(HostConsumer); ok {
					c.ConsumeHost(host)
				}
			case source.AWSECSFargateKind:
				if c, ok := consumer.(TagsConsumer); ok {
					c.ConsumeTag(src.Tag())
				}
			}
		}
	}
	return metadata, nil
}

// isUnsupportedMetric returns true if the input metric is not consumable by OTLP metrics intake endpoint.
// Unsupported OTLP metrics include cumulative monotonic sum metrics, cumulative histogram and exponential histogram metrics,
// metrics with an aggregation temporality other than delta or cumulative, and metrics with an empty metric type.
func isUnsupportedMetric(m pmetric.Metric) bool {
	aggr := pmetric.AggregationTemporalityUnspecified
	switch m.Type() {
	case pmetric.MetricTypeSum:
		aggr = m.Sum().AggregationTemporality()
		if aggr == pmetric.AggregationTemporalityCumulative && !m.Sum().IsMonotonic() {
			// Cumulative non-monotonic sum metrics are consumable as Gauges.
			return false
		}
	case pmetric.MetricTypeHistogram:
		aggr = m.Histogram().AggregationTemporality()
	case pmetric.MetricTypeExponentialHistogram:
		aggr = m.ExponentialHistogram().AggregationTemporality()
	case pmetric.MetricTypeSummary, pmetric.MetricTypeGauge:
		return false
	}
	return aggr != pmetric.AggregationTemporalityDelta
}

func (t *minimalTranslator) mapToDDFormat(ctx context.Context, md pmetric.Metric, consumer Consumer, additionalTags []string, host string, scopeName string, rattrs pcommon.Map) error {
	baseDims := &Dimensions{
		name:                md.Name(),
		tags:                additionalTags,
		host:                host,
		originID:            attributes.OriginIDFromAttributes(rattrs),
		originProduct:       t.cfg.originProduct,
		originSubProduct:    OriginSubProductOTLP,
		originProductDetail: originProductDetailFromScopeName(scopeName),
	}
	if isUnsupportedMetric(md) {
		// Skip unsupported metrics (cumulative monotonic sums, cumulative histograms)
		// instead of returning an error that would stop the entire translation
		t.logger.Debug("Skipping unsupported metric",
			zap.String(metricName, md.Name()),
			zap.String("type", md.Type().String()),
		)
		return nil
	}
	switch md.Type() {
	case pmetric.MetricTypeGauge:
		t.mapper.MapNumberMetrics(ctx, consumer, baseDims, Gauge, md.Gauge().DataPoints())
	case pmetric.MetricTypeSum:
		switch md.Sum().AggregationTemporality() {
		case pmetric.AggregationTemporalityCumulative:
			if isCumulativeMonotonic(md) {
				switch t.cfg.NumberMode {
				case NumberModeCumulativeToDelta:
				// Not supported, use raw value
				case NumberModeRawValue:
					t.mapper.MapNumberMetrics(ctx, consumer, baseDims, Gauge, md.Sum().DataPoints())
				}
			} else { // delta and cumulative non-monotonic sums
				t.mapper.MapNumberMetrics(ctx, consumer, baseDims, Gauge, md.Sum().DataPoints())
			}
		case pmetric.AggregationTemporalityDelta:
			t.mapper.MapNumberMetrics(ctx, consumer, baseDims, Count, md.Sum().DataPoints())
		default: // pmetric.AggregationTemporalityUnspecified or any other not supported type
			t.logger.Debug("Unknown or unsupported aggregation temporality",
				zap.String(metricName, md.Name()),
				zap.Any("aggregation temporality", md.Sum().AggregationTemporality()),
			)
		}
	case pmetric.MetricTypeHistogram:
		switch md.Histogram().AggregationTemporality() {
		case pmetric.AggregationTemporalityCumulative, pmetric.AggregationTemporalityDelta:
			err := t.mapper.MapHistogramMetrics(ctx, consumer, baseDims, md.Histogram().DataPoints(), true)
			if err != nil {
				return err
			}
		default: // pmetric.AggregationTemporalityUnspecified or any other not supported type
			t.logger.Debug("Unknown or unsupported aggregation temporality",
				zap.String("metric name", md.Name()),
				zap.Any("aggregation temporality", md.Histogram().AggregationTemporality()),
			)
		}
	case pmetric.MetricTypeExponentialHistogram:
		switch md.ExponentialHistogram().AggregationTemporality() {
		case pmetric.AggregationTemporalityDelta:
			delta := md.ExponentialHistogram().AggregationTemporality() == pmetric.AggregationTemporalityDelta
			t.mapper.MapExponentialHistogramMetrics(ctx, consumer, baseDims, md.ExponentialHistogram().DataPoints(), delta)
		default: // pmetric.AggregationTemporalityCumulative, pmetric.AggregationTemporalityUnspecified or any other not supported type
			t.logger.Debug("Unknown or unsupported aggregation temporality",
				zap.String("metric name", md.Name()),
				zap.Any("aggregation temporality", md.ExponentialHistogram().AggregationTemporality()),
			)
		}
	case pmetric.MetricTypeSummary:
		t.mapper.MapSummaryMetrics(ctx, consumer, baseDims, md.Summary().DataPoints())
	default: // pmetric.MetricDataTypeNone or any other not supported type
		t.logger.Debug("Unknown or unsupported metric type", zap.String(metricName, md.Name()), zap.Any("data type", md.Type()))
	}
	return nil
}
