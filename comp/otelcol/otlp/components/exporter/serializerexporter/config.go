// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializerexporter

import (
	"errors"
	"fmt"
	"strings"
	"time"

	datadogconfig "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/configopaque"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.uber.org/zap"
)

// ExporterConfig defines configuration for the serializer exporter.
type ExporterConfig struct {
	// squash ensures fields are correctly decoded in embedded struct
	exporterhelper.TimeoutConfig `mapstructure:",squash"`

	HTTPConfig confighttp.ClientConfig `mapstructure:",squash"`

	exporterhelper.QueueBatchConfig `mapstructure:"sending_queue"`

	configtls.ClientConfig `mapstructure:"tls"`

	Metrics MetricsConfig `mapstructure:"metrics"`
	// API defines the Datadog API configuration.
	// It is useful for OSS OpenTelemetry Collector or to use
	// the serializer exporter with the OCB.
	API datadogconfig.APIConfig `mapstructure:"api"`

	// HostProvider is the function to get the host name.
	// OpenTelemetry Collector provides a override for this.
	HostProvider SourceProviderFunc `mapstructure:"-"`

	// ShutdownFunc is the function to call when the exporter is shutdown.
	// OpenTelemetry Collector provides additional shutdown logic.
	ShutdownFunc component.ShutdownFunc `mapstructure:"-"`

	// HostMetadataConfig defines the host metadata related configuration.
	HostMetadata datadogconfig.HostMetadataConfig `mapstructure:"host_metadata"`

	// Non-fatal warnings found during configuration loading.
	warnings []error
}

// Validate the configuration for errors. This is required by component.Config.
func (c *ExporterConfig) Validate() error {
	key := string(c.API.Key)
	if key != "" { // there is no api key in the config when serializer exporter is used in OTLP ingest
		if err := datadogconfig.StaticAPIKeyCheck(key); err != nil {
			return err
		}
	}

	histCfg := c.Metrics.Metrics.HistConfig
	if histCfg.Mode == datadogconfig.HistogramModeNoBuckets && !histCfg.SendAggregations {
		return fmt.Errorf("'nobuckets' mode and `send_aggregation_metrics` set to false will send no histogram metrics")
	}

	if c.HostMetadata.Enabled && c.HostMetadata.ReporterPeriod < 5*time.Minute {
		return errors.New("reporter_period must be 5 minutes or higher")
	}

	return nil
}

// LogWarnings logs warning messages that were generated on unmarshaling.
func (c *ExporterConfig) LogWarnings(logger *zap.Logger) {
	for _, err := range c.warnings {
		logger.Warn(fmt.Sprintf("%v", err))
	}
}

// Unmarshal a configuration map into the configuration struct.
func (c *ExporterConfig) Unmarshal(configMap *confmap.Conf) error {
	err := configMap.Unmarshal(c)
	if err != nil {
		return err
	}

	if c.HostMetadata.HostnameSource == datadogconfig.HostnameSourceFirstResource {
		c.warnings = append(c.warnings, errors.New("first_resource is deprecated, opt in to https://docs.datadoghq.com/opentelemetry/mapping/host_metadata/ instead"))
	}

	c.API.Key = configopaque.String(strings.TrimSpace(string(c.API.Key)))

	// If an endpoint is not explicitly set, override it based on the site.
	if !configMap.IsSet("metrics::endpoint") {
		c.Metrics.Metrics.Endpoint = fmt.Sprintf("https://api.%s", c.API.Site)
	}

	// Return an error if an endpoint is explicitly set to ""
	if c.Metrics.Metrics.Endpoint == "" {
		return datadogconfig.ErrEmptyEndpoint
	}

	const (
		initialValueSetting = "metrics::sums::initial_cumulative_monotonic_value"
		cumulMonoMode       = "metrics::sums::cumulative_monotonic_mode"
	)
	if configMap.IsSet(initialValueSetting) && c.Metrics.Metrics.SumConfig.CumulativeMonotonicMode != datadogconfig.CumulativeMonotonicSumModeToDelta {
		return fmt.Errorf("%q can only be configured when %q is set to %q",
			initialValueSetting, cumulMonoMode, datadogconfig.CumulativeMonotonicSumModeToDelta)
	}

	return nil
}

var _ component.Config = (*ExporterConfig)(nil)
var _ confmap.Unmarshaler = (*ExporterConfig)(nil)

// MetricsConfig defines the metrics exporter specific configuration options
type MetricsConfig struct {
	Metrics datadogconfig.MetricsConfig `mapstructure:",squash"`

	// The following 2 configs are only used in OTLP ingestion and not expected to be used in the converged agent.

	// APMStatsReceiverAddr is the address to send APM stats to.
	APMStatsReceiverAddr string `mapstructure:"apm_stats_receiver_addr"`

	// Tags is a comma-separated list of tags to add to all metrics.
	Tags string `mapstructure:"tags"`
}
