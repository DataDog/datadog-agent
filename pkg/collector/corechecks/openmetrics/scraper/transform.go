// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scraper

import (
	"fmt"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// defaultMetricType is used when no explicit type is configured for a metric.
const defaultMetricType = "native"

// MetricTransformer routes metrics to the appropriate TransformerFunc based on
// configuration. It handles exact metric name matches, regex patterns (via
// MetricFilter), and "native" type resolution (using the endpoint-reported type).
type MetricTransformer struct {
	filter        *MetricFilter
	globalOptions HistogramOptions

	// Pre-compiled transformers for exact metric matches that have a fixed type.
	exact map[string]TransformerFunc

	// Cache for native-type metrics: the transformer is resolved on first
	// encounter from the endpoint-reported type and cached for subsequent scrapes.
	nativeMu sync.RWMutex
	nativeCache map[string]TransformerFunc

	// Summary transformer options
	sendDistSumsAsMonotonic bool
}

// NewMetricTransformer creates a MetricTransformer from the scraper Config.
func NewMetricTransformer(cfg *Config, filter *MetricFilter) *MetricTransformer {
	histOpts := HistogramOptions{
		CollectHistogramBuckets:          boolVal(cfg.CollectHistogramBuckets, true),
		HistogramBucketsAsDistributions:  cfg.HistogramBucketsAsDistributions,
		NonCumulativeHistogramBuckets:    boolVal(cfg.NonCumulativeHistogramBuckets, false),
		CollectCountersWithDistributions: cfg.CollectCountersWithDistributions,
	}

	mt := &MetricTransformer{
		filter:                  filter,
		globalOptions:           histOpts,
		exact:                   make(map[string]TransformerFunc),
		nativeCache:             make(map[string]TransformerFunc),
		sendDistSumsAsMonotonic: cfg.SendDistributionSumsAsMonotonic,
	}

	// Pre-compile transformers for exact-match metrics that have a non-native type.
	for rawName, inc := range filter.exactIncludes {
		if inc.match.Type != "" && inc.match.Type != defaultMetricType {
			name := inc.match.Name
			if name == "" {
				name = rawName
			}
			tf, err := mt.buildTransformer(inc.match.Type)
			if err != nil {
				log.Warnf("openmetrics: cannot build transformer for metric %q type %q: %v", rawName, inc.match.Type, err)
				continue
			}
			mt.exact[rawName] = tf
		}
	}

	return mt
}

// Get returns the TransformerFunc for a given raw metric name and the
// Prometheus-reported type (e.g., "COUNTER", "GAUGE"). If the metric is
// configured with a fixed type, that transformer is returned. Otherwise the
// endpoint-reported type is used ("native" resolution).
// Returns nil if no transformer can be determined.
func (mt *MetricTransformer) Get(rawName string, promType string) TransformerFunc {
	// Pre-compiled exact transformer?
	if tf, ok := mt.exact[rawName]; ok {
		return tf
	}

	// Native resolution: determine transformer from the Prometheus type.
	return mt.resolveNative(rawName, promType)
}

// resolveNative returns a transformer for a "native" metric by mapping the
// Prometheus-reported type to a Datadog transformer. Results are cached.
func (mt *MetricTransformer) resolveNative(rawName string, promType string) TransformerFunc {
	mt.nativeMu.RLock()
	if tf, ok := mt.nativeCache[rawName]; ok {
		mt.nativeMu.RUnlock()
		return tf
	}
	mt.nativeMu.RUnlock()

	typeLower := strings.ToLower(promType)
	tf, err := mt.buildTransformer(typeLower)
	if err != nil {
		log.Debugf("openmetrics: skipping metric %q with unsupported type %q", rawName, promType)
		return nil
	}

	mt.nativeMu.Lock()
	mt.nativeCache[rawName] = tf
	mt.nativeMu.Unlock()

	return tf
}

// buildTransformer returns a TransformerFunc for the given metric type name.
func (mt *MetricTransformer) buildTransformer(typeName string) (TransformerFunc, error) {
	switch typeName {
	case "gauge", "untyped":
		return newGaugeTransformer(), nil
	case "counter":
		return newCounterTransformer(), nil
	case "histogram":
		return newHistogramTransformer(mt.globalOptions), nil
	case "summary":
		return newSummaryTransformer(mt.sendDistSumsAsMonotonic), nil
	case "rate":
		return newRateTransformer(), nil
	case "counter_gauge":
		return newCounterGaugeTransformer(), nil
	default:
		return nil, fmt.Errorf("unknown metric type %q", typeName)
	}
}

// boolVal returns the value of a *bool, or the given default if nil.
func boolVal(b *bool, def bool) bool {
	if b != nil {
		return *b
	}
	return def
}
