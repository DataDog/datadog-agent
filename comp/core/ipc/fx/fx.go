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
		fx.Provide(newIPC),
		fx.Provide(newIPCClient),
		fx.Provide(newOptionalIPCClient),
	)
}

type optionalIPCComp struct {
	fx.In
	IPC option.Option[ipc.Component]
	Log log.Component
}

func newIPC(deps optionalIPCComp) (ipc.Component, error) {
	ipc, ok := deps.IPC.Get()
	if !ok {
		return nil, deps.Log.Errorf("ipc component has not been initialized")
	}
	return ipc, nil
}

// newIPCClient allow to use ipc.HTTPClient as dependency instead of using option.Option[ipc.Component].
func newIPCClient(deps optionalIPCComp) (ipc.HTTPClient, error) {
	ipc, ok := deps.IPC.Get()
	if !ok {
		return nil, deps.Log.Errorf("ipc client not found")
	}
	return ipc.GetClient(), nil
}

// newOptionalIPCClient allow to use option.Option[authtoken.IPCClient] as dependency instead of using option.Option[ipc.Component].
func newOptionalIPCClient(deps optionalIPCComp) option.Option[ipc.HTTPClient] {
	ipcComp, ok := deps.IPC.Get()
	if !ok {
		return option.None[ipc.HTTPClient]()
	}
	return option.New(ipcComp.GetClient())
}
