package serializerexporter

import (
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

// exporterConfig defines configuration for the serializer exporter.
type exporterConfig struct {
	// squash ensures fields are correctly decoded in embedded struct
	config.ExporterSettings        `mapstructure:",squash"`
	exporterhelper.TimeoutSettings `mapstructure:",squash"`
	exporterhelper.QueueSettings   `mapstructure:",squash"`

	Metrics metricsConfig `mapstructure:"metrics"`
}

// metricsConfig defines the metrics exporter specific configuration options
type metricsConfig struct {
	// Quantiles states whether to report quantiles from summary metrics.
	// By default, the minimum, maximum and average are reported.
	Quantiles bool `mapstructure:"report_quantiles"`

	// SendMonotonic states whether to report cumulative monotonic metrics as counters
	// or gauges
	SendMonotonic bool `mapstructure:"send_monotonic_counter"`

	// DeltaTTL defines the time that previous points of a cumulative monotonic
	// metric are kept in memory to calculate deltas
	DeltaTTL int64 `mapstructure:"delta_ttl"`

	ExporterConfig metricsExporterConfig `mapstructure:",squash"`

	// HistConfig defines the export of OTLP Histograms.
	HistConfig histogramConfig `mapstructure:"histograms"`
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

	// SendCountSum states if the export should send .sum and .count metrics for histograms.
	// The current default is false.
	SendCountSum bool `mapstructure:"send_count_sum_metrics"`
}

// metricsExporterConfig provides options for a user to customize the behavior of the
// metrics exporter
type metricsExporterConfig struct {
	// ResourceAttributesAsTags, if set to true, will use the exporterhelper feature to transform all
	// resource attributes into metric labels, which are then converted into tags
	ResourceAttributesAsTags bool `mapstructure:"resource_attributes_as_tags"`

	// InstrumentationLibraryMetadataAsTags, if set to true, adds the name and version of the
	// instrumentation library that created a metric to the metric tags
	InstrumentationLibraryMetadataAsTags bool `mapstructure:"instrumentation_library_metadata_as_tags"`
}
