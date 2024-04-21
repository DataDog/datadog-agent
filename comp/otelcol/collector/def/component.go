// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

// Package collector implements the OpenTelemetry Collector component.
package def

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/otelcol/otlp"
)

// team: opentelemetry

// Component specifies the interface implemented by the collector module.
type Component interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Status() otlp.CollectorStatus
}
