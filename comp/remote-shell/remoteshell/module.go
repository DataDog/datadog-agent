// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package remoteshell

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/remote-shell/remoteshell/def"
	"github.com/DataDog/datadog-agent/comp/remote-shell/remoteshell/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newRemoteShell),
	)
}

// Requires contains the dependencies needed to create the remote shell component
type Requires struct {
	fx.In

	Config config.Component
	Log    log.Component
}

// Provides contains the fields provided by the remote shell constructor
type Provides struct {
	fx.Out

	Comp def.Component
}

func newRemoteShell(req Requires) Provides {
	return Provides{
		Comp: impl.NewComponent(req.Config, req.Log),
	}
}
