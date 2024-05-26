// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

// Package collector implements the OpenTelemetry Collector component.
package collector

import (
	"go.uber.org/fx"

	collectordef "github.com/DataDog/datadog-agent/comp/otelcol/collector/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: opentelemetry

// Component specifies the interface implemented by the collector module.
type Component = collectordef.Component

// Module specifies the Collector module bundle.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newPipeline))
}
