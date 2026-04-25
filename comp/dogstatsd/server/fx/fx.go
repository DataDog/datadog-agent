// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the fx module for the dogstatsd server component.
package fx

import (
	"go.uber.org/fx"

	serverdef "github.com/DataDog/datadog-agent/comp/dogstatsd/server/def"
	serverimpl "github.com/DataDog/datadog-agent/comp/dogstatsd/server/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-metric-pipelines

// Module defines the fx options for this component.
func Module(params serverdef.Params) fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(serverimpl.NewComponent),
		fx.Supply(params),
	)
}
