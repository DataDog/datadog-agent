// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metrics

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics/internal/instrumentationlibrary"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics/internal/instrumentationscope"
)

const (
	metricName             string = "metric name"
	errNoBucketsNoSumCount string = "no buckets mode and no send count sum are incompatible"

	// intervalTolerance is the tolerance for interval calculation in seconds
	// We use 0.05 seconds as tolerance to allow for some jitter.
	intervalTolerance float64 = 0.05
)

var (
	signalTypeSet      = attribute.NewSet(attribute.String("signal", "metrics"))
	rateAsGaugeMetrics = map[string]struct{}{
		"kafka.net.bytes_out.rate":                        {},
		"kafka.net.bytes_in.rate":                         {},
		"kafka.replication.isr_shrinks.rate":              {},
		"kafka.replication.isr_expands.rate":              {},
		"kafka.replication.leader_elections.rate":         {},
		"jvm.gc.minor_collection_count":                   {},
		"jvm.gc.major_collection_count":                   {},
		"jvm.gc.minor_collection_time":                    {},
		"jvm.gc.major_collection_time":                    {},
		"kafka.messages_in.rate":                          {},
		"kafka.request.produce.failed.rate":               {},
		"kafka.request.fetch.failed.rate":                 {},
		"kafka.replication.unclean_leader_elections.rate": {},
		"kafka.log.flush_rate.rate":                       {},
	}
)

// inferDeltaInterval calculates the interval for Datadog counts from OTLP delta sums.
// It returns the interval in seconds if the time difference between start and end timestamps
// is close to a whole number of seconds (within intervalTolerance), otherwise returns 0.
func inferDeltaInterval(startTimestamp, timestamp uint64) int64 {
	if startTimestamp == 0 {
		// We can't infer the interval without the startTimestamp
		return 0
	}

	if startTimestamp > timestamp {
		// malformed data
		return 0
	}

	// Convert nanoseconds to seconds
	deltaSeconds := float64(timestamp-startTimestamp) / 1e9
	roundedDelta := math.Round(deltaSeconds)

	if math.Abs(roundedDelta-deltaSeconds) < intervalTolerance {
		return int64(roundedDelta)
	}

	// delta is outside of tolerance range
	return 0
}

var _ source.Provider = (*noSourceProvider)(nil)

type noSourceProvider struct{}

func (*noSourceProvider) Source(context.Context) (source.Source, error) {
	return source.Source{Kind: source.HostnameKind, Identifier: ""}, nil
}

// defaultTranslator is the default metrics translator implementation.
// It uses Consumer which includes both sketch and raw histogram consumers.
type defaultTranslator struct {
	prevPts              *ttlCache
	logger               *zap.Logger
	attributesTranslator *attributes.Translator
	cfg                  translatorConfig
	mapper               mapper
}

// NewDefaultTranslator creates a new translator with the given options.
// It returns a Provider interface that can be used for translating OTLP metrics.
// The returned translator also implements StatsTranslator for APM stats conversion.
func NewDefaultTranslator(set component.TelemetrySettings, attributesTranslator *attributes.Translator, options ...TranslatorOption) (Provider, error) {

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

	if cfg.HistMode == HistogramModeNoBuckets && !cfg.SendHistogramAggregations {
		return nil, errors.New(errNoBucketsNoSumCount)
	}

	cache := newTTLCache(cfg.sweepInterval, cfg.deltaTTL)
	logger := set.Logger.With(zap.String("component", "metrics translator"))

	t := &defaultTranslator{
		prevPts:              cache,
		logger:               logger,
		attributesTranslator: attributesTranslator,
		cfg:                  cfg,
	}
	// Use custom mapper if provided, otherwise create a defaultMapper
	if cfg.customMapper != nil {
		t.mapper = cfg.customMapper
		return t, nil
	}
	t.mapper = newDefaultMapper(cache, logger, cfg)
	return t, nil
}

// NewTranslator creates a new translator with given options.
//
// Deprecated: Use [NewDefaultTranslator] instead.
func NewTranslator(set component.TelemetrySettings, attributesTranslator *attributes.Translator, options ...TranslatorOption) (*Translator, error) {
	d, err := NewDefaultTranslator(set, attributesTranslator, options...)
	if err != nil {
		return nil, err
	}
	return &Translator{
		Provider: d,
		logger:   set.Logger,
	}, nil
}

// getMapper returns the underlying Mapper implementation.
// This is useful for testing or for direct access to mapping methods.
func (t *defaultTranslator) getMapper() mapper {
	return t.mapper
}

// mapper defines the interface for mapping OTLP metric data points to Datadog format.
// Consumer includes both sketch and raw histogram consumers - implementations can
// use either based on configuration. Consumers can no-op methods they don't need.
type mapper interface {
	MapNumberMetrics(
		ctx context.Context,
		consumer Consumer,
		dims *Dimensions,
		dt DataType,
		slice pmetric.NumberDataPointSlice,
	)

	MapSummaryMetrics(
		ctx context.Context,
		consumer Consumer,
		dims *Dimensions,
		slice pmetric.SummaryDataPointSlice,
	)

	MapHistogramMetrics(
		ctx context.Context,
		consumer Consumer,
		dims *Dimensions,
		slice pmetric.HistogramDataPointSlice,
		delta bool,
	) error

	MapExponentialHistogramMetrics(
		ctx context.Context,
		consumer Consumer,
		dims *Dimensions,
		slice pmetric.ExponentialHistogramDataPointSlice,
		delta bool,
	)
}

// Translator is provided for backward compatibility.
// External code using *metrics.Translator will continue to work.
//
// Deprecated: Use [NewDefaultTranslator] and [Provider] interface instead.
type Translator struct {
	Provider
	logger *zap.Logger
}

// Provider defines the interface for translating OTLP metrics to Datadog format.
type Provider interface {
	MapMetrics(ctx context.Context, md pmetric.Metrics, consumer Consumer, hostFromAttributesHandler attributes.HostFromAttributesHandler) (Metadata, error)
}

// Metadata specifies information about the outcome of the MapMetrics call.
type Metadata struct {
	// Languages specifies a list of languages for which runtime metrics were found.
	Languages []string
}

// isCumulativeMonotonic checks if a metric is a cumulative monotonic metric
func isCumulativeMonotonic(md pmetric.Metric) bool {
	switch md.Type() {
	case pmetric.MetricTypeSum:
		return md.Sum().AggregationTemporality() == pmetric.AggregationTemporalityCumulative &&
			md.Sum().IsMonotonic()
	}
	return false
}

// isSkippable checks if a value can be skipped (because it is not supported by the backend).
// It logs that the value is unsupported for debugging since this sometimes means there is a bug.
func isSkippable(logger *zap.Logger, name string, v float64) bool {
	skippable := math.IsInf(v, 0) || math.IsNaN(v)
	if skippable {
		logger.Debug("Unsupported metric value", zap.String(metricName, name), zap.Float64("value", v))
	}
	return skippable
}

// TODO(songy23): consider changing this to a Translator start time that must be initialized
// if the package-level variable causes any issue.
var startTime = uint64(time.Now().UnixNano())

// getProcessStartTime returns the start time of the Agent process in seconds since epoch
func getProcessStartTime() uint64 {
	return startTime / 1_000_000_000
}

// getProcessStartTimeNano returns the start time of the Agent process in nanoseconds since epoch
func getProcessStartTimeNano() uint64 {
	return startTime
}

// shouldConsumeInitialValue checks if the initial value of a cumulative monotonic metric
// should be consumed or dropped based on the configuration mode.
func shouldConsumeInitialValue(mode InitialCumulMonoValueMode, startTs, ts uint64) bool {
	switch mode {
	case InitialCumulMonoValueModeAuto:
		if getProcessStartTimeNano() < startTs && startTs != ts {
			// Report the first value if the timeseries started after the Datadog Agent process started.
			return true
		}
	case InitialCumulMonoValueModeKeep:
		return true
	case InitialCumulMonoValueModeDrop:
		// do nothing, drop the point
	}
	return false
}

// mapNumberMonotonicMetrics maps monotonic datapoints into Datadog metrics
func (t *defaultTranslator) mapNumberMonotonicMetrics(
	ctx context.Context,
	consumer TimeSeriesConsumer,
	dims *Dimensions,
	slice pmetric.NumberDataPointSlice,
) {
	for i := 0; i < slice.Len(); i++ {
		p := slice.At(i)
		if p.Flags().NoRecordedValue() {
			// No recorded value, skip.
			continue
		}

		ts := uint64(p.Timestamp())
		startTs := uint64(p.StartTimestamp())
		pointDims := dims.WithAttributeMap(p.Attributes())

		var val float64
		switch p.ValueType() {
		case pmetric.NumberDataPointValueTypeDouble:
			val = p.DoubleValue()
		case pmetric.NumberDataPointValueTypeInt:
			val = float64(p.IntValue())
		}

		if isSkippable(t.logger, pointDims.name, val) {
			continue
		}

		if _, ok := rateAsGaugeMetrics[pointDims.name]; ok {
			dx, isFirstPoint, shouldDropPoint := t.prevPts.MonotonicRate(pointDims, startTs, ts, val)
			if shouldDropPoint {
				t.logger.Debug("Dropping point: timestamp is older or equal to timestamp of previous point received", zap.String(metricName, pointDims.name))
			} else if !isFirstPoint {
				consumer.ConsumeTimeSeries(ctx, pointDims, Gauge, ts, 0, dx)
			}
			continue
		}

		dx, isFirstPoint, shouldDropPoint := t.prevPts.MonotonicDiff(pointDims, startTs, ts, val)
		if shouldDropPoint {
			t.logger.Debug("Dropping point: timestamp is older or equal to timestamp of previous point received", zap.String(metricName, pointDims.name))
			continue
		}

		if !isFirstPoint {
			consumer.ConsumeTimeSeries(ctx, pointDims, Count, ts, 0, dx)
		} else if i == 0 && shouldConsumeInitialValue(t.cfg.InitialCumulMonoValueMode, startTs, ts) {
			// We only compute the first point in the timeseries if it is the first value in the datapoint slice.
			// Todo: Investigate why we don't compute first val if i > 0 and add reason as comment.
			consumer.ConsumeTimeSeries(ctx, pointDims, Count, ts, 0, val)
		}
	}
}

type histogramInfo struct {
	// sum of histogram (exact)
	sum float64
	// count of histogram (exact)
	count uint64

	// hasMinFromLastTimeWindow indicates whether the minimum was reached in the last time window.
	// If the minimum is NOT available, its value is false.
	hasMinFromLastTimeWindow bool

	// hasMaxFromLastTimeWindow indicates whether the maximum was reached in the last time window.
	// If the maximum is NOT available, its value is false.
	hasMaxFromLastTimeWindow bool

	// ok to use sum/count.
	ok bool
}

// formatFloat formats a float number as close as possible to what
// we do on the Datadog Agent Python OpenMetrics check, which, in turn, tries to
// follow https://github.com/OpenObservability/OpenMetrics/blob/v1.0.0/specification/OpenMetrics.md#considerations-canonical-numbers
func formatFloat(f float64) string {
	if math.IsInf(f, 1) {
		return "inf"
	} else if math.IsInf(f, -1) {
		return "-inf"
	} else if math.IsNaN(f) {
		return "nan"
	} else if f == 0 {
		return "0"
	}

	// Add .0 to whole numbers
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if f == math.Floor(f) {
		s = s + ".0"
	}
	return s
}

// getQuantileTag returns the quantile tag for summary types.
func getQuantileTag(quantile float64) string {
	return "quantile:" + formatFloat(quantile)
}

// resolveSource determines the source from resource attributes, falling back to the fallbackSourceProvider if no source is found.
func resolveSource(ctx context.Context, attributesTranslator *attributes.Translator, res pcommon.Resource, fallbackSourceProvider source.Provider, hostFromAttributesHandler attributes.HostFromAttributesHandler) (source.Source, error) {
	src, hasSource := attributesTranslator.ResourceToSource(ctx, res, signalTypeSet, hostFromAttributesHandler)
	if !hasSource {
		var err error
		src, err = fallbackSourceProvider.Source(ctx)
		if err != nil {
			return source.Source{}, fmt.Errorf("failed to get fallback source: %w", err)
		}
	}
	return src, nil
}

// extractLanguageTag appends a new language tag to languageTags if a new language tag is found from the given name
func extractLanguageTag(name string, languageTags []string) []string {
	for prefix, lang := range runtimeMetricPrefixLanguageMap {
		if !slices.Contains(languageTags, lang) && strings.HasPrefix(name, prefix) {
			return append(languageTags, lang)
		}
	}
	return languageTags
}

// mapGaugeRuntimeMetricWithAttributes maps the specified runtime metric from metric attributes into a new Gauge metric
func mapGaugeRuntimeMetricWithAttributes(md pmetric.Metric, metricsArray pmetric.MetricSlice, mp runtimeMetricMapping) {
	for i := 0; i < md.Gauge().DataPoints().Len(); i++ {
		matchesAttributes := true
		for _, attribute := range mp.attributes {
			attributeValue, res := md.Gauge().DataPoints().At(i).Attributes().Get(attribute.key)
			if !res || !slices.Contains(attribute.values, attributeValue.AsString()) {
				matchesAttributes = false
				break
			}
		}
		if matchesAttributes {
			cp := metricsArray.AppendEmpty()
			cp.SetEmptyGauge()
			dataPoint := cp.Gauge().DataPoints().AppendEmpty()
			md.Gauge().DataPoints().At(i).CopyTo(dataPoint)
			dataPoint.Attributes().RemoveIf(func(s string, _ pcommon.Value) bool {
				for _, attribute := range mp.attributes {
					if s == attribute.key {
						return true
					}
				}
				return false
			})
			cp.SetName(mp.mappedName)
		}
	}
}

// mapSumRuntimeMetricWithAttributes maps the specified runtime metric from metric attributes into a new Sum metric
func mapSumRuntimeMetricWithAttributes(md pmetric.Metric, metricsArray pmetric.MetricSlice, mp runtimeMetricMapping) {
	for i := 0; i < md.Sum().DataPoints().Len(); i++ {
		matchesAttributes := true
		for _, attribute := range mp.attributes {
			attributeValue, res := md.Sum().DataPoints().At(i).Attributes().Get(attribute.key)
			if !res || !slices.Contains(attribute.values, attributeValue.AsString()) {
				matchesAttributes = false
				break
			}
		}
		if matchesAttributes {
			cp := metricsArray.AppendEmpty()
			cp.SetEmptySum()
			cp.Sum().SetAggregationTemporality(md.Sum().AggregationTemporality())
			cp.Sum().SetIsMonotonic(md.Sum().IsMonotonic())
			dataPoint := cp.Sum().DataPoints().AppendEmpty()
			md.Sum().DataPoints().At(i).CopyTo(dataPoint)
			dataPoint.Attributes().RemoveIf(func(s string, _ pcommon.Value) bool {
				for _, attribute := range mp.attributes {
					if s == attribute.key {
						return true
					}
				}
				return false
			})
			cp.SetName(mp.mappedName)
		}
	}
}

// mapHistogramRuntimeMetricWithAttributes maps the specified runtime metric from metric attributes into a new Histogram metric
func mapHistogramRuntimeMetricWithAttributes(md pmetric.Metric, metricsArray pmetric.MetricSlice, mp runtimeMetricMapping) {
	for i := 0; i < md.Histogram().DataPoints().Len(); i++ {
		matchesAttributes := true
		for _, attribute := range mp.attributes {
			attributeValue, res := md.Histogram().DataPoints().At(i).Attributes().Get(attribute.key)
			if !res || !slices.Contains(attribute.values, attributeValue.AsString()) {
				matchesAttributes = false
				break
			}
		}
		if matchesAttributes {
			cp := metricsArray.AppendEmpty()
			cp.SetEmptyHistogram()
			cp.Histogram().SetAggregationTemporality(md.Histogram().AggregationTemporality())
			dataPoint := cp.Histogram().DataPoints().AppendEmpty()
			md.Histogram().DataPoints().At(i).CopyTo(dataPoint)
			dataPoint.Attributes().RemoveIf(func(s string, _ pcommon.Value) bool {
				for _, attribute := range mp.attributes {
					if s == attribute.key {
						return true
					}
				}
				return false
			})
			cp.SetName(mp.mappedName)
			break
		}
	}
}

// MapMetrics maps OTLP metrics into the Datadog format
func (t *defaultTranslator) MapMetrics(ctx context.Context, md pmetric.Metrics, consumer Consumer, hostFromAttributesHandler attributes.HostFromAttributesHandler) (Metadata, error) {
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

			newMetrics := pmetric.NewMetricSlice()
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
				if v, ok := runtimeMetricsMappings[md.Name()]; ok {
					metadata.Languages = extractLanguageTag(md.Name(), metadata.Languages)
					for _, mp := range v {
						if mp.attributes == nil {
							// duplicate runtime metrics as Datadog runtime metrics
							cp := newMetrics.AppendEmpty()
							md.CopyTo(cp)
							cp.SetName(mp.mappedName)
							break
						}
						if md.Type() == pmetric.MetricTypeSum {
							mapSumRuntimeMetricWithAttributes(md, newMetrics, mp)
						} else if md.Type() == pmetric.MetricTypeGauge {
							mapGaugeRuntimeMetricWithAttributes(md, newMetrics, mp)
						} else if md.Type() == pmetric.MetricTypeHistogram {
							mapHistogramRuntimeMetricWithAttributes(md, newMetrics, mp)
						}
					}
				} else {
					// If we are here, we have a non-APM metric:
					// it is not a stats metric, nor a runtime metric.
					seenNonAPMMetrics = true
				}

				if t.cfg.withRemapping {
					remapMetrics(newMetrics, md)
				}
				if t.cfg.withOTelPrefix {
					renameMetrics(md)
				}

				err := t.mapToDDFormat(ctx, md, consumer, additionalTags, host, scopeName, rattrs)
				if err != nil {
					return metadata, err
				}
			}

			for k := 0; k < newMetrics.Len(); k++ {
				md := newMetrics.At(k)
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

func (t *defaultTranslator) mapToDDFormat(ctx context.Context, md pmetric.Metric, consumer Consumer, additionalTags []string, host string, scopeName string, rattrs pcommon.Map) error {
	baseDims := &Dimensions{
		name:                md.Name(),
		tags:                additionalTags,
		host:                host,
		originID:            attributes.OriginIDFromAttributes(rattrs),
		originProduct:       t.cfg.originProduct,
		originSubProduct:    OriginSubProductOTLP,
		originProductDetail: originProductDetailFromScopeName(scopeName),
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
					t.mapNumberMonotonicMetrics(ctx, consumer, baseDims, md.Sum().DataPoints())
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
			delta := md.Histogram().AggregationTemporality() == pmetric.AggregationTemporalityDelta
			err := t.mapper.MapHistogramMetrics(ctx, consumer, baseDims, md.Histogram().DataPoints(), delta)
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
