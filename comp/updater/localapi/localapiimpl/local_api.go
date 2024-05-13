// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package localapiimpl implements the installer local api component.
package localapiimpl

import (
	"fmt"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/updater/localapi"
	updatercomp "github.com/DataDog/datadog-agent/comp/updater/updater"
	"github.com/DataDog/datadog-agent/pkg/fleet/daemon"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module is the fx module for the updater local api.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newLocalAPIComponent),
	)
}

// dependencies contains the dependencies to build the updater local api.
type dependencies struct {
	fx.In

	Config  config.Component
	Updater updatercomp.Component
	Log     log.Component
}

func newLocalAPIComponent(lc fx.Lifecycle, deps dependencies) (localapi.Component, error) {
	localAPI, err := daemon.NewLocalAPI(deps.Updater, deps.Config.GetString("run_path"))
	if err != nil {
		return nil, fmt.Errorf("could not create local API: %w", err)
	}
	lc.Append(fx.Hook{OnStart: localAPI.Start, OnStop: localAPI.Stop})
	return localAPI, nil
}
