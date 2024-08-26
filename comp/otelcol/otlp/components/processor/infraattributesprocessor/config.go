// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"go.opentelemetry.io/collector/component"
)

// COPIED FROM comp/core/tagger/types.go
// TagCardinality indicates the cardinality-level of a tag.
// It can be low cardinality (in the host count order of magnitude)
// orchestrator cardinality (tags that change value for each pod, task, etc.)
// high cardinality (typically tags that change value for each web request, each container, etc.)
type TagCardinality int

// List of possible container cardinality
const (
	LowCardinality TagCardinality = iota
	OrchestratorCardinality
	HighCardinality
)

// Config defines configuration for processor.
type Config struct {
	Metrics MetricInfraAttributes `mapstructure:"metrics"`
	Logs    LogInfraAttributes    `mapstructure:"logs"`
	Traces  TraceInfraAttributes  `mapstructure:"traces"`

	Cardinality TagCardinality `mapstructure:"cardinality"`
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
