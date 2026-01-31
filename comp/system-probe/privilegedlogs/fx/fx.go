// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package fx provides the fx module for the privilegedlogs component
package fx

import (
	privilegedlogs "github.com/DataDog/datadog-agent/comp/system-probe/privilegedlogs/def"
	privilegedlogsimpl "github.com/DataDog/datadog-agent/comp/system-probe/privilegedlogs/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			privilegedlogsimpl.NewComponent,
		),
		fxutil.ProvideOptional[privilegedlogs.Component](),
	)
}
