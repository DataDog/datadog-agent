// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx defines the fx options for the flare component.
package fx

import (
	"go.uber.org/fx"

	flare "github.com/DataDog/datadog-agent/comp/core/flare/def"
	flareimpl "github.com/DataDog/datadog-agent/comp/core/flare/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module(params flare.Params) fxutil.Module {
	return fxutil.Component(
		fx.Supply(params),
		fxutil.ProvideComponentConstructor(flareimpl.NewComponent),
	)
}
