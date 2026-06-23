// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package fx provides the fx module for the defaultforwarder component.
package fx

import (
	"go.uber.org/fx"

	defaultforwarderdef "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	defaultforwarderimpl "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module(params defaultforwarderdef.Params) fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(defaultforwarderimpl.NewComponent),
		fx.Supply(params),
	)
}

// ModuleWithOptionTMP defines the fx options with a temporary option.
// Deprecated: will be removed once configsync cleanup is done.
func ModuleWithOptionTMP(option fx.Option) fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(defaultforwarderimpl.NewComponent),
		option,
	)
}
