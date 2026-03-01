// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package apiserver initializes the api server that powers many subcommands.
// Deprecated: import from comp/process/apiserver/def, comp/process/apiserver/fx,
// or comp/process/apiserver/mock instead.
package apiserver

import (
	apiserverdef "github.com/DataDog/datadog-agent/comp/process/apiserver/def"
	apiserverimpl "github.com/DataDog/datadog-agent/comp/process/apiserver/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: container-experiences

// Component is the component type.
// Deprecated: Use comp/process/apiserver/def.Component directly.
//
//nolint:revive // TODO(PROC) Fix revive linter
type Component = apiserverdef.Component

// Module defines the fx options for this component.
// Deprecated: Use comp/process/apiserver/fx.Module() instead.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			apiserverimpl.NewComponent,
		),
	)
}
