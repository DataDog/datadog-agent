// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the ipc component
package fx

import (
	ipcimpl "github.com/DataDog/datadog-agent/comp/core/ipc/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// ModuleForDaemon defines the fx options for this component for the daemon commands.
// It is using the NewReadWriteComponent constructor under the hood.
func ModuleForDaemon() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			ipcimpl.NewReadWriteComponent,
		),
	)
}

// ModuleForOneshot defines the fx options for this component for the one-shot commands.
// It is using the NewReadOnlyComponent constructor under the hood.
func ModuleForOneshot() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			ipcimpl.NewReadOnlyComponent,
		),
	)
}

// ModuleForDebug defines the fx options for this component for commands that should work
// even if the agent is not running or IPC artifacts are not initialized.
// WARNING: This module should not be used outside of the flare and diagnose commands.
// This module covers cases where it is acceptable to not have initialized IPC component.
// This is typically the case for commands that MUST work no matter the coreAgent is running or not, if the auth artifacts are not found/initialized.
// A good example is the `flare` command, which should return a flare even if the IPC component is not initialized.
func ModuleForDebug() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			ipcimpl.NewDebugOnlyComponent,
		),
	)
}
