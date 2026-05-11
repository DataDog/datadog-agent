// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package openmetrics

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
)

const (
	transformerNative        = "native"
	transformerNativeDynamic = "native_dynamic"
	transformerLegacy        = "legacy"
)

type runtimeData struct {
	flushFirstValue bool
	staticTags      []string
}

type sampleDatum struct {
	sample   parsedSample
	tags     []string
	hostname string
}

type transformerFunc func(parsedMetric, []sampleDatum, runtimeData, sender.Sender)

type compiledTransformer struct {
	metricType  string
	transformer transformerFunc
}

type metricPattern struct {
	pattern *regexp.Regexp
	config  map[string]interface{}
}

type metricTransformer struct {
	cfg                 *scraperConfig
	exact               map[string]*compiledTransformer
	patterns            []metricPattern
	cacheMetricWildcard bool
}

func newMetricTransformer(cfg *scraperConfig) (*metricTransformer, error) {
	transformer := &metricTransformer{
		cfg:                 cfg,
		exact:               map[string]*compiledTransformer{},
		cacheMetricWildcard: cfg.cacheMetricWildcards,
	}

	metricsConfig, err := normalizeMetricConfig(cfg)
	if err != nil {
		return nil, err
	}

	for rawMetricName, metricConfig := range metricsConfig {
		quoted := regexp.QuoteMeta(rawMetricName)
		if rawMetricName != quoted {
			pattern, err := regexp.Compile(rawMetricName)
			if err != nil {
				return nil, err
			}
			patternConfig := copyConfigMap(metricConfig)
			delete(patternConfig, "name")
			transformer.patterns = append(transformer.patterns, metricPattern{pattern: pattern, config: patternConfig})
			continue
		}

		compiled, err := transformer.compile(metricConfig)
		if err != nil {
			return nil, fmt.Errorf("error compiling transformer for metric `%s`: %w", rawMetricName, err)
		}
		transformer.exact[rawMetricName] = compiled
	}

	return transformer, nil
}

func (t *metricTransformer) get(metric parsedMetric) *compiledTransformer {
	if transformer := t.exact[metric.Name]; transformer != nil {
		if transformer.metricType == transformerNative && skipNativeMetric(metric) {
			return nil
		}
		return transformer
	}

	for _, pattern := range t.patterns {
		if !pattern.pattern.MatchString(metric.Name) {
			continue
		}
		config := map[string]interface{}{"name": metric.Name}
		for key, value := range pattern.config {
			config[key] = value
		}
		transformer, err := t.compile(config)
		if err != nil {
			return nil
		}
		if t.cacheMetricWildcard {
			t.exact[metric.Name] = transformer
		}
		if transformer.metricType == transformerNative && skipNativeMetric(metric) {
			return nil
		}
		return transformer
	}

	return nil
}

func (t *metricTransformer) compile(config map[string]interface{}) (*compiledTransformer, error) {
	metricName, ok := config["name"].(string)
	if !ok {
		return nil, errors.New("field `name` must be a string")
	}

	metricType, ok := config["type"].(string)
	if !ok {
		return nil, errors.New("field `type` must be a string")
	}
	rawMetricName := metricName
	metricName = t.metricName(metricName)

	switch metricType {
	case transformerNative:
		return &compiledTransformer{metricType: metricType, transformer: t.native(metricName)}, nil
	case transformerNativeDynamic:
		return &compiledTransformer{metricType: metricType, transformer: t.nativeDynamic(metricName)}, nil
	case transformerLegacy:
		return &compiledTransformer{metricType: metricType, transformer: t.legacy(metricName, config)}, nil
	case "counter":
		return &compiledTransformer{metricType: metricType, transformer: submitCounter(metricName)}, nil
	case "gauge":
		return &compiledTransformer{metricType: metricType, transformer: submitGauge(metricName)}, nil
	case "histogram":
		return &compiledTransformer{metricType: metricType, transformer: t.submitHistogram(metricName)}, nil
	case "summary":
		return &compiledTransformer{metricType: metricType, transformer: submitSummary(metricName)}, nil
	case "counter_gauge":
		return &compiledTransformer{metricType: metricType, transformer: submitCounterGauge(metricName)}, nil
	case "rate":
		return &compiledTransformer{metricType: metricType, transformer: submitRate(metricName)}, nil
	case "service_check":
		statusMap, err := compileStatusMap(config["status_map"])
		if err != nil {
			return nil, err
		}
		return &compiledTransformer{metricType: metricType, transformer: submitServiceCheck(metricName, statusMap)}, nil
	case "temporal_percent":
		scale, err := compileScale(config["scale"])
		if err != nil {
			return nil, err
		}
		return &compiledTransformer{metricType: metricType, transformer: submitTemporalPercent(metricName, scale)}, nil
	case "time_elapsed":
		return &compiledTransformer{metricType: metricType, transformer: submitTimeElapsed(metricName)}, nil
	case "metadata":
		rawLabel, ok := config["label"]
		if !ok {
			return nil, errors.New("the `label` parameter is required")
		}
		label, ok := rawLabel.(string)
		if !ok {
			return nil, errors.New("the `label` parameter must be a string")
		}
		if label == "" {
			return nil, errors.New("the `label` parameter is required")
		}
		options := copyConfigMap(config)
		delete(options, "name")
		delete(options, "type")
		delete(options, "label")
		return &compiledTransformer{metricType: metricType, transformer: submitMetadata(rawMetricName, label, options, t.cfg.checkID)}, nil
	case "legacy_metadata":
		labelMap, err := compileMetadataLabelMap(config["label_map"])
		if err != nil {
			return nil, err
		}
		return &compiledTransformer{metricType: metricType, transformer: submitLegacyMetadata(labelMap, t.cfg.checkID)}, nil
	default:
		return nil, fmt.Errorf("unknown type `%s`", metricType)
	}
}

func (t *metricTransformer) metricName(name string) string {
	if t.cfg.namespace == "" || name == "" {
		return name
	}
	return strings.TrimSuffix(t.cfg.namespace, ".") + "." + strings.TrimPrefix(name, ".")
}

func (t *metricTransformer) native(metricName string) transformerFunc {
	var transformer transformerFunc
	return func(metric parsedMetric, samples []sampleDatum, runtime runtimeData, sender sender.Sender) {
		if transformer == nil {
			transformer = t.nativeTransformer(metric.Type, metricName)
		}
		if transformer != nil {
			transformer(metric, samples, runtime, sender)
		}
	}
}

func (t *metricTransformer) nativeDynamic(metricName string) transformerFunc {
	cached := map[string]transformerFunc{}
	return func(metric parsedMetric, samples []sampleDatum, runtime runtimeData, sender sender.Sender) {
		transformer := cached[metric.Type]
		if transformer == nil {
			transformer = t.nativeTransformer(metric.Type, metricName)
			cached[metric.Type] = transformer
		}
		if transformer != nil {
			transformer(metric, samples, runtime, sender)
		}
	}
}

func (t *metricTransformer) nativeTransformer(metricType, metricName string) transformerFunc {
	switch metricType {
	case "counter":
		return submitCounter(metricName)
	case "gauge":
		return submitGauge(metricName)
	case "histogram":
		return t.submitHistogram(metricName)
	case "summary":
		return submitSummary(metricName)
	default:
		return nil
	}
}

func skipNativeMetric(metric parsedMetric) bool {
	switch metric.Type {
	case "counter", "gauge", "histogram", "summary":
		return false
	default:
		return true
	}
}

func submitCounter(metricName string) transformerFunc {
	metricName += ".count"
	return func(_ parsedMetric, samples []sampleDatum, runtime runtimeData, sender sender.Sender) {
		for _, sample := range samples {
			sender.MonotonicCountWithFlushFirstValue(metricName, sample.sample.Value, sample.hostname, sample.tags, runtime.flushFirstValue)
		}
	}
}

func submitGauge(metricName string) transformerFunc {
	return func(_ parsedMetric, samples []sampleDatum, _ runtimeData, sender sender.Sender) {
		for _, sample := range samples {
			sender.Gauge(metricName, sample.sample.Value, sample.hostname, sample.tags)
		}
	}
}

func submitCounterGauge(metricName string) transformerFunc {
	totalMetric := metricName + ".total"
	countMetric := metricName + ".count"
	return func(_ parsedMetric, samples []sampleDatum, runtime runtimeData, sender sender.Sender) {
		for _, sample := range samples {
			sender.Gauge(totalMetric, sample.sample.Value, sample.hostname, sample.tags)
			sender.MonotonicCountWithFlushFirstValue(countMetric, sample.sample.Value, sample.hostname, sample.tags, runtime.flushFirstValue)
		}
	}
}

func submitRate(metricName string) transformerFunc {
	return func(_ parsedMetric, samples []sampleDatum, _ runtimeData, sender sender.Sender) {
		for _, sample := range samples {
			sender.Rate(metricName, sample.sample.Value, sample.hostname, sample.tags)
		}
	}
}

func submitSummary(metricName string) transformerFunc {
	sumMetric := metricName + ".sum"
	countMetric := metricName + ".count"
	quantileMetric := metricName + ".quantile"
	return func(metric parsedMetric, samples []sampleDatum, runtime runtimeData, sender sender.Sender) {
		for _, sample := range samples {
			switch {
			case strings.HasSuffix(sample.sample.Name, "_sum"):
				sender.MonotonicCountWithFlushFirstValue(sumMetric, sample.sample.Value, sample.hostname, sample.tags, runtime.flushFirstValue)
			case strings.HasSuffix(sample.sample.Name, "_count"):
				sender.MonotonicCountWithFlushFirstValue(countMetric, sample.sample.Value, sample.hostname, sample.tags, runtime.flushFirstValue)
			case sample.sample.Name == metric.Name:
				sender.Gauge(quantileMetric, sample.sample.Value, sample.hostname, sample.tags)
			}
		}
	}
}

func (t *metricTransformer) submitHistogram(metricName string) transformerFunc {
	if !t.cfg.collectHistogramBuckets {
		sumMetric := metricName + ".sum"
		countMetric := metricName + ".count"
		return func(_ parsedMetric, samples []sampleDatum, runtime runtimeData, sender sender.Sender) {
			for _, sample := range samples {
				switch {
				case strings.HasSuffix(sample.sample.Name, "_sum"):
					sender.MonotonicCountWithFlushFirstValue(sumMetric, sample.sample.Value, sample.hostname, sample.tags, runtime.flushFirstValue)
				case strings.HasSuffix(sample.sample.Name, "_count"):
					sender.MonotonicCountWithFlushFirstValue(countMetric, sample.sample.Value, sample.hostname, sample.tags, runtime.flushFirstValue)
				}
			}
		}
	}

	if t.cfg.histogramBucketsAsDistributions {
		sumMetric := metricName + ".sum"
		countMetric := metricName + ".count"
		return func(_ parsedMetric, samples []sampleDatum, runtime runtimeData, sender sender.Sender) {
			for _, sample := range decumulateHistogramBuckets(samples) {
				switch {
				case strings.HasSuffix(sample.sample.Name, "_sum") && t.cfg.collectCountersWithDistributions:
					sender.MonotonicCountWithFlushFirstValue(sumMetric, sample.sample.Value, sample.hostname, sample.tags, runtime.flushFirstValue)
				case strings.HasSuffix(sample.sample.Name, "_count") && t.cfg.collectCountersWithDistributions:
					sender.MonotonicCountWithFlushFirstValue(countMetric, sample.sample.Value, sample.hostname, sample.tags, runtime.flushFirstValue)
				case strings.HasSuffix(sample.sample.Name, "_bucket"):
					lowerBound, lowerErr := strconv.ParseFloat(sample.sample.Labels["lower_bound"], 64)
					upperBound, upperErr := strconv.ParseFloat(sample.sample.Labels["upper_bound"], 64)
					if lowerErr != nil || upperErr != nil || lowerBound == upperBound {
						continue
					}
					sender.HistogramBucket(metricName, int64(sample.sample.Value), lowerBound, upperBound, true, sample.hostname, sample.tags, runtime.flushFirstValue)
				}
			}
		}
	}

	bucketMetric := metricName + ".bucket"
	sumMetric := metricName + ".sum"
	countMetric := metricName + ".count"
	return func(_ parsedMetric, samples []sampleDatum, runtime runtimeData, sender sender.Sender) {
		if t.cfg.nonCumulativeHistogramBuckets {
			samples = decumulateHistogramBuckets(samples)
		}
		for _, sample := range samples {
			switch {
			case strings.HasSuffix(sample.sample.Name, "_sum"):
				sender.MonotonicCountWithFlushFirstValue(sumMetric, sample.sample.Value, sample.hostname, sample.tags, runtime.flushFirstValue)
			case strings.HasSuffix(sample.sample.Name, "_count"):
				sender.MonotonicCountWithFlushFirstValue(countMetric, sample.sample.Value, sample.hostname, sample.tags, runtime.flushFirstValue)
			case strings.HasSuffix(sample.sample.Name, "_bucket") && !strings.HasSuffix(sample.sample.Labels["upper_bound"], "inf"):
				sender.MonotonicCountWithFlushFirstValue(bucketMetric, sample.sample.Value, sample.hostname, sample.tags, runtime.flushFirstValue)
			}
		}
	}
}

func (t *metricTransformer) legacy(metricName string, config map[string]interface{}) transformerFunc {
	override, _ := config["legacy_type_override"].(string)
	return func(metric parsedMetric, samples []sampleDatum, runtime runtimeData, sender sender.Sender) {
		metricType := metric.Type
		if override != "" {
			metricType = override
		}
		switch metricType {
		case "counter":
			for _, sample := range samples {
				if t.cfg.sendMonotonicCounter {
					sender.MonotonicCountWithFlushFirstValue(metricName, sample.sample.Value, sample.hostname, sample.tags, runtime.flushFirstValue)
				} else {
					sender.Gauge(metricName, sample.sample.Value, sample.hostname, sample.tags)
					if t.cfg.sendMonotonicWithGauge {
						sender.MonotonicCountWithFlushFirstValue(metricName+".total", sample.sample.Value, sample.hostname, sample.tags, runtime.flushFirstValue)
					}
				}
			}
		case "histogram":
			t.legacyHistogram(metricName, samples, runtime, sender)
		case "summary":
			t.legacySummary(metricName, samples, runtime, sender)
		case "gauge":
			for _, sample := range samples {
				sender.Gauge(metricName, sample.sample.Value, sample.hostname, sample.tags)
			}
		}
	}
}

func (t *metricTransformer) legacyHistogram(metricName string, samples []sampleDatum, runtime runtimeData, sender sender.Sender) {
	if t.cfg.sendDistributionBuckets {
		for _, sample := range decumulateHistogramBuckets(samples) {
			if !strings.HasSuffix(sample.sample.Name, "_bucket") {
				continue
			}
			lowerBound, lowerErr := strconv.ParseFloat(sample.sample.Labels["lower_bound"], 64)
			upperBound, upperErr := strconv.ParseFloat(sample.sample.Labels["upper_bound"], 64)
			if lowerErr != nil || upperErr != nil || lowerBound == upperBound || math.IsInf(upperBound, 1) {
				continue
			}
			sender.HistogramBucket(metricName, int64(sample.sample.Value), lowerBound, upperBound, true, sample.hostname, sample.tags, runtime.flushFirstValue)
		}
		return
	}

	for _, sample := range samples {
		switch {
		case strings.HasSuffix(sample.sample.Name, "_sum") && !t.cfg.sendDistributionBuckets:
			t.sendLegacyDistributionCount(metricName+".sum", sample, t.cfg.sendDistributionSumsAsMonotonic, runtime, sender)
		case strings.HasSuffix(sample.sample.Name, "_count") && !t.cfg.sendDistributionBuckets:
			if t.cfg.sendHistogramBuckets {
				sample.tags = append(copyStringSlice(sample.tags), "upper_bound:none")
			}
			t.sendLegacyDistributionCount(metricName+".count", sample, t.cfg.sendDistributionCountsAsMonotonic, runtime, sender)
		case strings.HasSuffix(sample.sample.Name, "_bucket") && t.cfg.sendHistogramBuckets && !strings.Contains(sample.sample.Labels["upper_bound"], "Inf") && !strings.Contains(sample.sample.Labels["upper_bound"], "inf"):
			t.sendLegacyDistributionCount(metricName+".count", sample, t.cfg.sendDistributionCountsAsMonotonic, runtime, sender)
		}
	}
}

func (t *metricTransformer) legacySummary(metricName string, samples []sampleDatum, runtime runtimeData, sender sender.Sender) {
	for _, sample := range samples {
		switch {
		case strings.HasSuffix(sample.sample.Name, "_sum"):
			t.sendLegacyDistributionCount(metricName+".sum", sample, t.cfg.sendDistributionSumsAsMonotonic, runtime, sender)
		case strings.HasSuffix(sample.sample.Name, "_count"):
			t.sendLegacyDistributionCount(metricName+".count", sample, t.cfg.sendDistributionCountsAsMonotonic, runtime, sender)
		default:
			sender.Gauge(metricName+".quantile", sample.sample.Value, sample.hostname, sample.tags)
		}
	}
}

func (t *metricTransformer) sendLegacyDistributionCount(metricName string, sample sampleDatum, monotonic bool, runtime runtimeData, sender sender.Sender) {
	if monotonic {
		sender.MonotonicCountWithFlushFirstValue(metricName, sample.sample.Value, sample.hostname, sample.tags, runtime.flushFirstValue)
		return
	}
	sender.Gauge(metricName, sample.sample.Value, sample.hostname, sample.tags)
	if t.cfg.sendMonotonicWithGauge {
		sender.MonotonicCountWithFlushFirstValue(metricName+".total", sample.sample.Value, sample.hostname, sample.tags, runtime.flushFirstValue)
	}
}

func submitServiceCheck(metricName string, statusMap map[int]servicecheck.ServiceCheckStatus) transformerFunc {
	return func(_ parsedMetric, samples []sampleDatum, runtime runtimeData, sender sender.Sender) {
		for _, sample := range samples {
			status, ok := statusMap[int(sample.sample.Value)]
			if !ok {
				status = servicecheck.ServiceCheckUnknown
			}
			sender.ServiceCheck(metricName, status, sample.hostname, runtime.staticTags, "")
		}
	}
}

func submitTemporalPercent(metricName string, scale int) transformerFunc {
	return func(_ parsedMetric, samples []sampleDatum, _ runtimeData, sender sender.Sender) {
		for _, sample := range samples {
			sender.Rate(metricName, sample.sample.Value*100/float64(scale), sample.hostname, sample.tags)
		}
	}
}

func submitTimeElapsed(metricName string) transformerFunc {
	return func(_ parsedMetric, samples []sampleDatum, _ runtimeData, sender sender.Sender) {
		now := float64(time.Now().Unix())
		for _, sample := range samples {
			sender.Gauge(metricName, now-sample.sample.Value, sample.hostname, sample.tags)
		}
	}
}

func decumulateHistogramBuckets(samples []sampleDatum) []sampleDatum {
	buckets := map[string]map[float64]float64{}
	for _, sample := range samples {
		if !strings.HasSuffix(sample.sample.Name, "_bucket") {
			continue
		}
		upperBound, err := strconv.ParseFloat(sample.sample.Labels["upper_bound"], 64)
		if err != nil {
			continue
		}
		context := labelContext(sample.sample.Labels, "upper_bound")
		if buckets[context] == nil {
			buckets[context] = map[float64]float64{}
		}
		buckets[context][upperBound] = sample.sample.Value
	}

	boundsByContext := make(map[string][]float64, len(buckets))
	for context, values := range buckets {
		for upperBound := range values {
			boundsByContext[context] = append(boundsByContext[context], upperBound)
		}
		sortFloat64s(boundsByContext[context])
	}

	out := make([]sampleDatum, 0, len(samples))
	for _, sample := range samples {
		if !strings.HasSuffix(sample.sample.Name, "_bucket") {
			out = append(out, sample)
			continue
		}
		upperBound, err := strconv.ParseFloat(sample.sample.Labels["upper_bound"], 64)
		if err != nil {
			out = append(out, sample)
			continue
		}
		context := labelContext(sample.sample.Labels, "upper_bound")
		bounds := boundsByContext[context]
		index := indexFloat64(bounds, upperBound)
		if index < 0 {
			out = append(out, sample)
			continue
		}

		lowerBound := math.Inf(-1)
		value := sample.sample.Value
		if index == 0 {
			if upperBound > 0 {
				lowerBound = 0
			}
		} else {
			lowerBound = bounds[index-1]
			value -= buckets[context][lowerBound]
		}

		decumulated := sample
		decumulated.sample.Labels = copyLabels(sample.sample.Labels)
		decumulated.sample.Value = value
		lowerBoundLabel := pythonFloatString(lowerBound)
		decumulated.sample.Labels["lower_bound"] = lowerBoundLabel
		decumulated.tags = append(copyStringSlice(sample.tags), "lower_bound:"+lowerBoundLabel)
		out = append(out, decumulated)
	}
	return out
}

func pythonFloatString(value float64) string {
	if value == 0 {
		return "0"
	}
	if math.IsInf(value, 1) {
		return "inf"
	}
	if math.IsInf(value, -1) {
		return "-inf"
	}
	if value == math.Trunc(value) {
		return strconv.FormatFloat(value, 'f', 1, 64)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func sortFloat64s(values []float64) {
	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}

func indexFloat64(values []float64, value float64) int {
	for i, candidate := range values {
		if candidate == value {
			return i
		}
	}
	return -1
}

func compileStatusMap(raw interface{}) (map[int]servicecheck.ServiceCheckStatus, error) {
	if raw == nil {
		return nil, errors.New("the `status_map` parameter is required")
	}
	rawMap, ok := normalizeMap(raw)
	if !ok {
		return nil, errors.New("the `status_map` parameter must be a mapping")
	}
	if len(rawMap) == 0 {
		return nil, errors.New("the `status_map` parameter must not be empty")
	}

	statuses := make(map[int]servicecheck.ServiceCheckStatus, len(rawMap))
	for rawValue, rawStatus := range rawMap {
		value, err := strconv.Atoi(rawValue)
		if err != nil {
			return nil, fmt.Errorf("value `%v` of parameter `status_map` does not represent an integer", rawValue)
		}
		statusString, ok := rawStatus.(string)
		if !ok {
			return nil, fmt.Errorf("status `%v` for value `%v` of parameter `status_map` is not a string", rawStatus, rawValue)
		}
		switch strings.ToUpper(statusString) {
		case "OK":
			statuses[value] = servicecheck.ServiceCheckOK
		case "WARNING":
			statuses[value] = servicecheck.ServiceCheckWarning
		case "CRITICAL":
			statuses[value] = servicecheck.ServiceCheckCritical
		case "UNKNOWN":
			statuses[value] = servicecheck.ServiceCheckUnknown
		default:
			return nil, fmt.Errorf("invalid status `%s` for value `%v` of parameter `status_map`", statusString, rawValue)
		}
	}
	return statuses, nil
}

func compileScale(raw interface{}) (int, error) {
	if raw == nil {
		return 0, errors.New("the `scale` parameter is required")
	}
	if scale, ok := raw.(int); ok {
		return scale, nil
	}
	if scale, ok := raw.(string); ok {
		switch strings.ToLower(scale) {
		case "second":
			return 1, nil
		case "millisecond":
			return 1000, nil
		case "microsecond":
			return 1000000, nil
		case "nanosecond":
			return 1000000000, nil
		default:
			return 0, errors.New("the `scale` parameter must be one of: microsecond | millisecond | nanosecond | second")
		}
	}
	return 0, errors.New("the `scale` parameter must be an integer representing parts of a second e.g. 1000 for millisecond")
}
