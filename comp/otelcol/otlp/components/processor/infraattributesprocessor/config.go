// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"go.opentelemetry.io/collector/component"
)

// Config defines configuration for processor.
type Config struct {
	Metrics MetricInfraAttributes `mapstructure:"metrics"`
	Logs    LogInfraAttributes    `mapstructure:"logs"`
	Traces  TraceInfraAttributes  `mapstructure:"traces"`
}

// MetricInfraAttributes - configuration for metrics.
type MetricInfraAttributes struct {
	MetricInfraAttributes []string `mapstructure:"metric"`
}

// TraceInfraAttributes - configuration for trace spans.
type TraceInfraAttributes struct {
	SpanInfraAttributes []string `mapstructure:"span"`
}

// LogInfraAttributes - configuration for logs.
type LogInfraAttributes struct {
	LogInfraAttributes []string `mapstructure:"log"`
}

var _ component.Config = (*Config)(nil)

// Validate configuration
func (cfg *Config) Validate() error {
	return nil
}
