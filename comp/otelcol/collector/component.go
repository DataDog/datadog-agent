// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package collector implements the OpenTelemetry Collector component.
package collector

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/otlp"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: opentelemetry

// Component specifies the interface implemented by the collector module.
type Component interface {
	Start() error
	Stop()
	Status() otlp.CollectorStatus
}

// Module specifies the Collector module bundle.
var Module = fxutil.Component(
	fx.Provide(newPipeline),
)
