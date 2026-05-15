// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the fx module for the live reporter component.
// Wire this in agent binaries: it provides the StdoutReporter + optional EventReporter.
package fx

import (
	reporterimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the live reporter component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			reporterimpl.NewComponent,
		),
	)
}
