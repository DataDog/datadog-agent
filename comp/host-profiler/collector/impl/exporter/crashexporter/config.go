// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package crashexporter

import "go.opentelemetry.io/collector/component"

// Config holds crash exporter settings.
type Config struct {
	// Site is the Datadog site (e.g. datadoghq.com). Used to derive the intake
	// URL when Endpoint is empty.
	Site string `mapstructure:"site"`

	// Endpoint overrides the full intake URL. Supports "stdout" and
	// "file:///path" for testing.
	Endpoint string `mapstructure:"endpoint"`

	// APIKey is the Datadog API key. Falls back to DD_API_KEY env var.
	APIKey string `mapstructure:"api_key"`
}

var _ component.Config = (*Config)(nil)

func (c *Config) Validate() error { return nil }
