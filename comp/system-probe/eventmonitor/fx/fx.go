// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || windows

// Package fx provides the fx module for the eventmonitor component
package fx

import (
	eventmonitor "github.com/DataDog/datadog-agent/comp/system-probe/eventmonitor/def"
	eventmonitorimpl "github.com/DataDog/datadog-agent/comp/system-probe/eventmonitor/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			eventmonitorimpl.NewComponent,
		),
		fxutil.ProvideOptional[eventmonitor.Component](),
	)
}
