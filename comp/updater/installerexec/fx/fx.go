// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the installerexec component
package fx

import (
	installerexec "github.com/DataDog/datadog-agent/comp/updater/installerexec/def"
	installerexecimpl "github.com/DataDog/datadog-agent/comp/updater/installerexec/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			installerexecimpl.NewComponent,
		),
		fxutil.ProvideOptional[installerexec.Component](),
	)
}
