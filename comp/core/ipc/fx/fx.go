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

// ModuleReadWrite defines the fx options for this component for the daemon commands.
// It will try to read the auth artifacts from the configured location.
// If they are not found, it will create them.
func ModuleReadWrite() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			ipcimpl.NewReadWriteComponent,
		),
	)
}

// ModuleReadOnly defines the fx options for this component for the one-shot commands.
// It will try to read the auth artifacts from the configured location.
// If they are not found, it will fail.
func ModuleReadOnly() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			ipcimpl.NewReadOnlyComponent,
		),
	)
}

// ModuleInsecure provides fx options for the IPC component suitable for specific commands
// (like 'flare' or 'diagnose') that must function even when the main Agent isn't running
// or IPC artifacts (like auth tokens) are missing or invalid.
//
// The component constructor provided by this module *always* succeeds, unlike
// ModuleReadWrite or ModuleReadOnly which might fail if artifacts are absent or incorrect.
// However, the resulting IPC component instance might be non-functional or only partially
// functional, potentially leading to failures later, such as rejected connections during
// the IPC handshake if communication with the core Agent is attempted.
//
// WARNING: This module is intended *only* for edge cases like diagnostics and flare generation.
// Using it in standard agent processes or commands that rely on stable IPC communication
// will likely lead to unexpected runtime errors or security issues.
func ModuleInsecure() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			ipcimpl.NewInsecureComponent,
		),
	)
}
