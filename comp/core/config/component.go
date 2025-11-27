// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config implements a component to handle agent configuration.  This
// component temporarily wraps pkg/config.
//
// This component initializes pkg/config based on the bundle params, and
// will return the same results as that package.  This is to support migration
// to a component architecture.  When no code still uses pkg/config, that
// package will be removed.
package config

import (
	"go.uber.org/fx"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-configuration

// Component is the component type.
type Component interface {
	pkgconfigmodel.ReaderWriter

	// Warnings returns config warnings collected during setup.
	Warnings() *pkgconfigmodel.Warnings
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newComponent))
}
