// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the fx module for the sdsscanner component.
package fx

import (
	sdsscanner "github.com/DataDog/datadog-agent/comp/core/sdsscanner/def"
	sdsscannerimpl "github.com/DataDog/datadog-agent/comp/core/sdsscanner/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			sdsscannerimpl.NewComponent,
		),
		fxutil.ProvideOptional[sdsscanner.Component](),
	)
}
