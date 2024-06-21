// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package collector defines the OpenTelemetry Collector component.
package collector

import (
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/datatype"
	"go.opentelemetry.io/collector/confmap"
)

// team: opentelemetry

// Component specifies the interface implemented by the collector module.
type Component interface {
	Status() datatype.CollectorStatus
	GetProvidedConf() (*confmap.Conf, error)
	GetEnhancedConf() (*confmap.Conf, error)
	GetProvidedConfAsString() (string, error)
	GetEnhancedConfAsString() (string, error)
}
