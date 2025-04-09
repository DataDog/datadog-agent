// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the ipc component
package fx

import (
	"go.uber.org/fx"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcimpl "github.com/DataDog/datadog-agent/comp/core/ipc/impl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			ipcimpl.NewComponent,
		),
		fx.Provide(unwrapIPCComp),
	)
}

type optionalIPCComp struct {
	fx.In
	At  option.Option[ipc.Component]
	Log log.Component
}

func unwrapIPCComp(deps optionalIPCComp) (ipc.Component, error) {
	ipc, ok := deps.At.Get()
	if !ok {
		return nil, deps.Log.Errorf("ipc component has not been initialized")
	}
	return ipc, nil
}
