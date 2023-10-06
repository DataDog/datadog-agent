// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializerexporter

import (
	"encoding"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

// exporterConfig defines configuration for the serializer exporter.
type exporterConfig struct {
	// squash ensures fields are correctly decoded in embedded struct
	exporterhelper.TimeoutSettings `mapstructure:",squash"`
	exporterhelper.QueueSettings   `mapstructure:",squash"`

	Metrics metricsConfig `mapstructure:"metrics"`

	warnings []string
}

var _ component.Config = (*exporterConfig)(nil)

// metricsConfig defines the metrics exporter specific configuration options
type metricsConfig struct {
	// Enabled reports whether Metrics should be enabled.
	Enabled bool `mapstructure:"enabled"`

	// DeltaTTL defines the time that previous points of a cumulative monotonic
	// metric are kept in memory to calculate deltas
	DeltaTTL int64 `mapstructure:"delta_ttl"`

	ExporterConfig metricsExporterConfig `mapstructure:",squash"`

	// TagCardinality is the level of granularity of tags to send for OTLP metrics.
	TagCardinality string `mapstructure:"tag_cardinality"`

	// HistConfig defines the export of OTLP Histograms.
	HistConfig histogramConfig `mapstructure:"histograms"`

	// SumConfig defines the export of OTLP Sums.
	SumConfig sumConfig `mapstructure:"sums"`

	// SummaryConfig defines the export for OTLP Summaries.
	SummaryConfig summaryConfig `mapstructure:"summaries"`
}

// histogramConfig customizes export of OTLP Histograms.
type histogramConfig struct {
	// Mode for exporting histograms. Valid values are 'distributions', 'counters' or 'nobuckets'.
	//  - 'distributions' sends histograms as Datadog distributions (recommended).
	//  - 'counters' sends histograms as Datadog counts, one metric per bucket.
	//  - 'nobuckets' sends no bucket histogram metrics. .sum and .count metrics will still be sent
	//    if `send_count_sum_metrics` is enabled.
	//
	// The current default is 'distributions'.
	Mode string `mapstructure:"mode"`

	// SendCountSum states if the export should send .sum, .count, .min and .max metrics for histograms.
	// The default is false.
	// Deprecated: use `send_aggregation_metrics` instead.
	SendCountSum bool `mapstructure:"send_count_sum_metrics"`

	// SendAggregations states if the export should send .sum, .count, .min and .max metrics for histograms.
	// The default is false.
	SendAggregations bool `mapstructure:"send_aggregation_metrics"`
}

// CumulativeMonotonicSumMode is the export mode for OTLP Sum metrics.
type CumulativeMonotonicSumMode string

const (
	// CumulativeMonotonicSumModeToDelta calculates delta for
	// cumulative monotonic sum metrics in the client side and reports
	// them as Datadog counts.
	CumulativeMonotonicSumModeToDelta CumulativeMonotonicSumMode = "to_delta"

	// CumulativeMonotonicSumModeRawValue reports the raw value for
	// cumulative monotonic sum metrics as a Datadog gauge.
	CumulativeMonotonicSumModeRawValue CumulativeMonotonicSumMode = "raw_value"
)

var _ encoding.TextUnmarshaler = (*CumulativeMonotonicSumMode)(nil)

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (sm *CumulativeMonotonicSumMode) UnmarshalText(in []byte) error {
	switch mode := CumulativeMonotonicSumMode(in); mode {
	case CumulativeMonotonicSumModeToDelta,
		CumulativeMonotonicSumModeRawValue:
		*sm = mode
		return nil
	default:
		return fmt.Errorf("invalid cumulative monotonic sum mode %q", mode)
	}
}

// InitialValueMode defines what the exporter should do with the initial value
// of a time series when transforming from cumulative to delta.
type InitialValueMode string

const (
	// InitialValueModeAuto reports the initial value if its start timestamp
	// is set and it happens after the process was started.
	InitialValueModeAuto InitialValueMode = "auto"

	// InitialValueModeDrop always drops the initial value.
	InitialValueModeDrop InitialValueMode = "drop"

	// InitialValueModeKeep always reports the initial value.
	InitialValueModeKeep InitialValueMode = "keep"
)

var _ encoding.TextUnmarshaler = (*InitialValueMode)(nil)

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (iv *InitialValueMode) UnmarshalText(in []byte) error {
	switch mode := InitialValueMode(in); mode {
	case InitialValueModeAuto,
		InitialValueModeDrop,
		InitialValueModeKeep:
		*iv = mode
		return nil
	default:
		return fmt.Errorf("invalid initial value mode %q", mode)
	}
}

// sumConfig customizes export of OTLP Sums.
type sumConfig struct {
	// CumulativeMonotonicMode is the mode for exporting OTLP Cumulative Monotonic Sums.
	// Valid values are 'to_delta' or 'raw_value'.
	//  - 'to_delta' calculates delta for cumulative monotonic sums and sends it as a Datadog count.
	//  - 'raw_value' sends the raw value of cumulative monotonic sums as Datadog gauges.
	//
	// The default is 'to_delta'.
	// See https://docs.datadoghq.com/metrics/otlp/?tab=sum#mapping for details and examples.
	CumulativeMonotonicMode CumulativeMonotonicSumMode `mapstructure:"cumulative_monotonic_mode"`

	// InitialCumulativeMonotonicMode defines the behavior of the exporter when receiving the first value
	// of a cumulative monotonic sum.
	InitialCumulativeMonotonicMode InitialValueMode `mapstructure:"initial_cumulative_monotonic_value"`
}

// SummaryMode is the export mode for OTLP Summary metrics.
type SummaryMode string

const (
	// SummaryModeNoQuantiles sends no `.quantile` metrics. `.sum` and `.count` metrics will still be sent.
	SummaryModeNoQuantiles SummaryMode = "noquantiles"
	// SummaryModeGauges sends `.quantile` metrics as gauges tagged by the quantile.
	SummaryModeGauges SummaryMode = "gauges"
)

var _ encoding.TextUnmarshaler = (*SummaryMode)(nil)

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (sm *SummaryMode) UnmarshalText(in []byte) error {
	switch mode := SummaryMode(in); mode {
	case SummaryModeNoQuantiles,
		SummaryModeGauges:
		*sm = mode
		return nil
	default:
		return fmt.Errorf("invalid summary mode %q", mode)
	}
}

// summaryConfig customizes export of OTLP Summaries.
type summaryConfig struct {
	// Mode is the the mode for exporting OTLP Summaries.
	// Valid values are 'noquantiles' or 'gauges'.
	//  - 'noquantiles' sends no `.quantile` metrics. `.sum` and `.count` metrics will still be sent.
	//  - 'gauges' sends `.quantile` metrics as gauges tagged by the quantile.
	//
	// The default is 'gauges'.
	// See https://docs.datadoghq.com/metrics/otlp/?tab=summary#mapping for details and examples.
	Mode SummaryMode `mapstructure:"mode"`
}

// metricsExporterConfig provides options for a user to customize the behavior of the
// metrics exporter
type metricsExporterConfig struct {
	// ResourceAttributesAsTags, if set to true, will use the exporterhelper feature to transform all
	// resource attributes into metric labels, which are then converted into tags
	ResourceAttributesAsTags bool `mapstructure:"resource_attributes_as_tags"`

	// Deprecated: Use InstrumentationScopeMetadataAsTags favor of in favor of
	// https://github.com/open-telemetry/opentelemetry-proto/releases/tag/v0.15.0
	// Both must not be set at the same time.
	// InstrumentationLibraryMetadataAsTags, if set to true, adds the name and version of the
	// instrumentation library that created a metric to the metric tags
	InstrumentationLibraryMetadataAsTags bool `mapstructure:"instrumentation_library_metadata_as_tags"`

	// InstrumentationScopeMetadataAsTags, if set to true, adds the name and version of the
	// instrumentation scope that created a metric to the metric tags
	InstrumentationScopeMetadataAsTags bool `mapstructure:"instrumentation_scope_metadata_as_tags"`
}

// Validate configuration
func (e *exporterConfig) Validate() error {
	return e.QueueSettings.Validate()
}

var _ confmap.Unmarshaler = (*exporterConfig)(nil)

const (
	warnDeprecatedSendCountSum   = "otlp_config.metrics.histograms.send_count_sum_metrics is deprecated in favor of otlp_config.metrics.histograms.send_aggregation_metrics"
	warnOverrideSendAggregations = "Overriding otlp_config.metrics.histograms.send_aggregation_metrics with otlp_config.metrics.histograms.send_count_sum_metrics value (deprecated)"
)

// Unmarshal a configuration map into the configuration struct.
func (e *exporterConfig) Unmarshal(configMap *confmap.Conf) error {
	err := configMap.Unmarshal(e, confmap.WithErrorUnused())
	if err != nil {
		return err
	}

	if configMap.IsSet("metrics::histograms::send_count_sum_metrics") {
		// send_count_sum_metrics is deprecated, warn the user
		e.warnings = append(e.warnings, warnDeprecatedSendCountSum)

		// override the value since send_count_sum_metrics was set
		e.Metrics.HistConfig.SendAggregations = e.Metrics.HistConfig.SendCountSum

		// if the user explicitly also set send_aggregation_metrics, warn that we overrided the value
		if configMap.IsSet("metrics::histograms::send_aggregation_metrics") {
			e.warnings = append(e.warnings, warnOverrideSendAggregations)
		}
	}

	const (
		initialValueSetting = "metrics::sums::initial_cumulative_monotonic_value"
		cumulMonoMode       = "metrics::sums::cumulative_monotonic_mode"
	)
	if configMap.IsSet(initialValueSetting) && e.Metrics.SumConfig.CumulativeMonotonicMode != CumulativeMonotonicSumModeToDelta {
		return fmt.Errorf("%q can only be configured when %q is set to %q",
			initialValueSetting, cumulMonoMode, CumulativeMonotonicSumModeToDelta)
	}

	return nil
}
