// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

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

type MetricTagEnrichment struct {
	MetricTagEnrichment []string `mapstructure:"metric"`
}

type TraceTagEnrichment struct {
	SpanTagEnrichment []string `mapstructure:"span"`
}

type LogTagEnrichment struct {
	LogTagEnrichment []string `mapstructure:"log"`
}

var _ component.Config = (*Config)(nil)

// Validate checks if the processor configuration is valid
func (cfg *Config) Validate() error {
	var errors error
	return errors
}
