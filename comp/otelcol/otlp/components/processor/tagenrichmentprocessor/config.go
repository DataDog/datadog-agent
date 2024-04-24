// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package tagenrichmentprocessor

import (
	"go.opentelemetry.io/collector/component"
)

// Config defines configuration for processor.
type Config struct {
	Metrics MetricTagEnrichment `mapstructure:"metrics"`
	Logs    LogTagEnrichment    `mapstructure:"logs"`
	Traces  TraceTagEnrichment  `mapstructure:"traces"`
}

// MetricTagEnrichment - configuration for metrics.
type MetricTagEnrichment struct {
	MetricTagEnrichment []string `mapstructure:"metric"`
}

// TraceTagEnrichment - configuration for trace spans.
type TraceTagEnrichment struct {
	SpanTagEnrichment []string `mapstructure:"span"`
}

// LogTagEnrichment - configuration for logs.
type LogTagEnrichment struct {
	LogTagEnrichment []string `mapstructure:"log"`
}

var _ component.Config = (*Config)(nil)

// Validate configuration
func (cfg *Config) Validate() error {
	var errors error
	return errors
}
