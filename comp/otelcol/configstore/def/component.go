// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package configstore defines the otel agent configstore component.
package configstore

import (
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/otelcol"
)

// team: opentelemetry

// Component provides functions to store and expose the provided and enhanced configs.
type Component interface {
	AddConfigs(otelcol.ConfigProviderSettings, otelcol.ConfigProviderSettings, otelcol.Factories) error
	GetProvidedConf() (*confmap.Conf, error)
	GetEnhancedConf() (*confmap.Conf, error)
	GetProvidedConfAsString() (string, error)
	GetEnhancedConfAsString() (string, error)
}
