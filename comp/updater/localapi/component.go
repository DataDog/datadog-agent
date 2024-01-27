// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package localapi implements the updater local api component.
package localapi

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/updater"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// team: fleet

// Module is the fx module for the updater local api.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newLocalAPIComponent),
	)
}

// Params contains the parameters to build the updater.
type Params struct {
	fx.In

	Updater *updater.Updater
	Log     log.Component
}

func newLocalAPIComponent(params Params) (*updater.LocalAPI, error) {
	localAPI, err := updater.NewLocalAPI(params.Updater)
	if err != nil {
		return nil, fmt.Errorf("could not create local API: %w", err)
	}
	return localAPI, nil
}

// Component is the interface for the updater local api component.
type Component interface {
}
