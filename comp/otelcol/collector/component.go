// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

// Package collector implements the OpenTelemetry Collector component.
package collector

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/otlp"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: opentelemetry

// TODO: This component can't use the fx lifecycle hooks for starting and stopping
// because it depends on the logs agent component's log channel which isn't ready at
// that time and can't be obtained. This needs to be addressed as an improvement
// of the logs agent component.

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
