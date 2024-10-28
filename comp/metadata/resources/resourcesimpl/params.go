// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resourcesimpl

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Params defines the parameters for the metadata resources component.
type Params struct {
	// Disabled determines if the resources payload will be sent. When disabled, the Get method is still available.
	Disabled bool
}

// Disabled returns a new Params struct that will disabled sending the resources payload.
func Disabled() *Params {
	return &Params{
		Disabled: true,
	}
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newResourcesProvider))
}
