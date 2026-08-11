// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the fx module for the installer metadata component
package fx

import (
	installer "github.com/DataDog/datadog-agent/comp/metadata/installer/def"
	installerimpl "github.com/DataDog/datadog-agent/comp/metadata/installer/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			installerimpl.NewComponent,
		),
		fxutil.ProvideOptional[installer.Component](),
	)
}
