// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package fx provides the standard telemetry component for fx-based applications.
package fx

import (
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-runtimes

// Module provides the standard telemetry implementation.
// This provides telemetry.Component (the base interface) backed by the prometheus implementation.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			telemetryimpl.NewComponent,
		),
	)
}
