// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the remoteflags component.
package fx

import (
	remoteflags "github.com/DataDog/datadog-agent/comp/core/remoteflags/def"
	remoteflagsimpl "github.com/DataDog/datadog-agent/comp/core/remoteflags/impl"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Module defines the fx options for the Remote Flags component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(remoteflagsimpl.NewComponent),
		fx.Provide(newRCListener),
	)
}

func newRCListener(comp remoteflags.Component) types.ListenerProvider {
	var rcListener types.ListenerProvider
	rcListener.ListenerProvider = types.RCListener{
		data.ProductAgentFlags: comp.GetClient().OnUpdate,
	}
	return rcListener
}
