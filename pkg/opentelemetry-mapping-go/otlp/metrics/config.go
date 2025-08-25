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
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
)

type translatorConfig struct {
	// metrics export behavior
	HistMode                             HistogramMode
	SendHistogramAggregations            bool
	Quantiles                            bool
	NumberMode                           NumberMode
	InitialCumulMonoValueMode            InitialCumulMonoValueMode
	InstrumentationLibraryMetadataAsTags bool
	InstrumentationScopeMetadataAsTags   bool
	InferDeltaInterval                   bool

	originProduct OriginProduct

	// withRemapping reports whether certain metrics that are only available when using
	// the Datadog Agent should be obtained by remapping from OTEL counterparts (e.g.
	// container.* and system.* metrics). This configuration also enables withOTelPrefix.
	withRemapping bool

	// withOTelPrefix reports whether some OpenTelemetry metrics (ex: host metrics) should be
	// renamed with the `otel.` prefix. This prevents the Collector and Datadog
	// Agent from computing metrics with the same names.
	withOTelPrefix bool

	// cache configuration
	sweepInterval int64
	deltaTTL      int64

	fallbackSourceProvider source.Provider
	// statsOut is the channel where the translator will send its APM statsPayload bytes
	statsOut chan<- []byte
}

// TranslatorOption is a translator creation option.
type TranslatorOption func(*translatorConfig) error

// WithRemapping specifies that certain OTEL metrics (such as container.* and system.*) need to be
// remapped to their Datadog counterparts because they will not be available otherwise. This happens
// in situations when the translator is running as part of a Collector without the Datadog Agent.
//
// Do note that in some scenarios this process renames certain metrics (such as for example prefixing
// system.* and process.* metrics with the otel.* namespace).
func WithRemapping() TranslatorOption {
	return func(t *translatorConfig) error {
		t.withRemapping = true
		// to maintain backward compatibility with the old remapping logic
		// withRemapping must rename some otel metrics
		t.withOTelPrefix = true
		return nil
	}
}

// WithOTelPrefix appends the `otel.` prefix to OpenTelemetry system, process and a subset of Kafka metrics.
func WithOTelPrefix() TranslatorOption {
	return func(t *translatorConfig) error {
		t.withOTelPrefix = true
		return nil
	}
}

// WithDeltaTTL sets the delta TTL for cumulative metrics datapoints.
// By default, 3600 seconds are used.
func WithDeltaTTL(deltaTTL int64) TranslatorOption {
	return func(t *translatorConfig) error {
		if deltaTTL <= 0 {
			return fmt.Errorf("time to live must be positive: %d", deltaTTL)
		}
		t.deltaTTL = deltaTTL
		t.sweepInterval = 1
		if t.deltaTTL > 1 {
			t.sweepInterval = t.deltaTTL / 2
		}
		return nil
	}
}

// WithFallbackSourceProvider sets the fallback source provider.
// By default, an empty hostname is used as a fallback.
func WithFallbackSourceProvider(provider source.Provider) TranslatorOption {
	return func(t *translatorConfig) error {
		t.fallbackSourceProvider = provider
		return nil
	}
}

// WithQuantiles enables quantiles exporting for summary metrics.
func WithQuantiles() TranslatorOption {
	return func(t *translatorConfig) error {
		t.Quantiles = true
		return nil
	}
}

// WithInstrumentationLibraryMetadataAsTags sets instrumentation library metadata as tags.
func WithInstrumentationLibraryMetadataAsTags() TranslatorOption {
	return func(t *translatorConfig) error {
		t.InstrumentationLibraryMetadataAsTags = true
		return nil
	}
}

// WithInstrumentationScopeMetadataAsTags sets instrumentation scope metadata as tags.
func WithInstrumentationScopeMetadataAsTags() TranslatorOption {
	return func(t *translatorConfig) error {
		t.InstrumentationScopeMetadataAsTags = true
		return nil
	}
}

// HistogramMode is an export mode for OTLP Histogram metrics.
type HistogramMode string

const (
	// HistogramModeNoBuckets disables bucket export.
	HistogramModeNoBuckets HistogramMode = "nobuckets"
	// HistogramModeCounters exports buckets as Datadog counts.
	HistogramModeCounters HistogramMode = "counters"
	// HistogramModeDistributions exports buckets as Datadog distributions.
	HistogramModeDistributions HistogramMode = "distributions"
)

// WithHistogramMode sets the histograms mode.
// The default mode is HistogramModeOff.
func WithHistogramMode(mode HistogramMode) TranslatorOption {
	return func(t *translatorConfig) error {
		switch mode {
		case HistogramModeNoBuckets, HistogramModeCounters, HistogramModeDistributions:
			t.HistMode = mode
		default:
			return fmt.Errorf("unknown histogram mode: %q", mode)
		}
		return nil
	}
}

// WithCountSumMetrics exports .count and .sum histogram metrics.
// Deprecated: Use WithHistogramAggregations instead.
func WithCountSumMetrics() TranslatorOption {
	return WithHistogramAggregations()
}

// WithHistogramAggregations exports .count, .sum, .min and .max histogram metrics when available.
func WithHistogramAggregations() TranslatorOption {
	return func(t *translatorConfig) error {
		t.SendHistogramAggregations = true
		return nil
	}
}

// WithOriginProduct sets the origin product attribute.
func WithOriginProduct(originProduct OriginProduct) TranslatorOption {
	return func(t *translatorConfig) error {
		t.originProduct = originProduct
		return nil
	}
}

// NumberMode is an export mode for OTLP Number metrics.
type NumberMode string

const (
	// NumberModeCumulativeToDelta calculates delta for
	// cumulative monotonic metrics in the client side and reports
	// them as Datadog counts.
	NumberModeCumulativeToDelta NumberMode = "cumulative_to_delta"

	// NumberModeRawValue reports the raw value for cumulative monotonic
	// metrics as a Datadog gauge.
	NumberModeRawValue NumberMode = "raw_value"
)

// WithNumberMode sets the number mode.
// The default mode is NumberModeCumulativeToDelta.
func WithNumberMode(mode NumberMode) TranslatorOption {
	return func(t *translatorConfig) error {
		t.NumberMode = mode
		return nil
	}
}

// WithStatsOut sets the channel where the translator will send its APM statsPayload bytes
func WithStatsOut(statsOut chan<- []byte) TranslatorOption {
	return func(t *translatorConfig) error {
		t.statsOut = statsOut
		return nil
	}
}

// InitialCumulMonoValueMode defines what the exporter should do with the initial value
// of a cumulative monotonic sum when under the 'cumulative_to_delta' mode.
// It also affects the count field for summary metrics.
// It is not used for cumulative monotonic sums when the mode is 'raw_value'.
type InitialCumulMonoValueMode string

const (
	// InitialCumulMonoValueModeAuto reports the initial value if its start timestamp
	// is set and it happens after the process was started.
	InitialCumulMonoValueModeAuto InitialCumulMonoValueMode = "auto"

	// InitialCumulMonoValueModeDrop always drops the initial value.
	InitialCumulMonoValueModeDrop InitialCumulMonoValueMode = "drop"

	// InitialCumulMonoValueModeKeep always reports the initial value.
	InitialCumulMonoValueModeKeep InitialCumulMonoValueMode = "keep"
)

// WithInitialCumulMonoValueMode sets the initial value mode.
// The default mode is InitialCumulMonoValueModeAuto.
func WithInitialCumulMonoValueMode(mode InitialCumulMonoValueMode) TranslatorOption {
	return func(t *translatorConfig) error {
		t.InitialCumulMonoValueMode = mode
		return nil
	}
}

// WithInferDeltaInterval infers the interval for delta sums.
// By default the interval is set to 0.
func WithInferDeltaInterval() TranslatorOption {
	return func(t *translatorConfig) error {
		t.InferDeltaInterval = true
		return nil
	}
}
