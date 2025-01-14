// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx provides the fx module for the ownerdetection component
package fx

import (
	//"go.uber.org/fx"

	ownerdetection "github.com/DataDog/datadog-agent/comp/core/ownerdetection/def"
	ownerdetectionimpl "github.com/DataDog/datadog-agent/comp/core/ownerdetection/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			ownerdetectionimpl.NewComponent,
		),
		//fx.Supply(params),
		fxutil.ProvideOptional[ownerdetection.Component](),
	)
}
