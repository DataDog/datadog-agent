// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package fxfull provides the full prometheus telemetry component for fx-based applications.
// This module provides impl.Component which includes prometheus-specific methods like
// RegisterCollector, UnregisterCollector, and Gather.
package fxfull

import (
	"github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-runtimes

// Module provides the full prometheus telemetry implementation.
// This provides impl.Component (the extended interface with prometheus-specific methods).
// Use this when you need RegisterCollector, UnregisterCollector, or Gather methods.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			impl.NewComponent,
		),
	)
}
