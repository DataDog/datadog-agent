// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializerexporter

import (
	datadogconfig "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

// ExporterConfig defines configuration for the serializer exporter.
type ExporterConfig struct {
	// squash ensures fields are correctly decoded in embedded struct
	exporterhelper.TimeoutConfig `mapstructure:",squash"`
	exporterhelper.QueueConfig   `mapstructure:",squash"`

	Metrics MetricsConfig `mapstructure:"metrics"`
}

var _ component.Config = (*ExporterConfig)(nil)

// MetricsConfig defines the metrics exporter specific configuration options
type MetricsConfig struct {
	Metrics datadogconfig.MetricsConfig `mapstructure:",squash"`

	// The following 3 configs are only used in OTLP ingestion and not expected to be used in the converged agent.

	// TagCardinality is the level of granularity of tags to send for OTLP metrics.
	TagCardinality string `mapstructure:"tag_cardinality"`

	// APMStatsReceiverAddr is the address to send APM stats to.
	APMStatsReceiverAddr string `mapstructure:"apm_stats_receiver_addr"`

	// Tags is a comma-separated list of tags to add to all metrics.
	Tags string `mapstructure:"tags"`
}
