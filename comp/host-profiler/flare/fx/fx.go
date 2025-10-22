// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx creates the modules for fx
package fx

import (
	flareimpl "github.com/DataDog/datadog-agent/comp/host-profiler/flare/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module for Flare in core agent
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			flareimpl.NewComponent,
		),
	)
}
