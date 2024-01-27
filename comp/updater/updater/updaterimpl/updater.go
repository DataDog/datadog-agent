// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package updater implements the updater component.
package updaterimpl

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	updatercomp "github.com/DataDog/datadog-agent/comp/updater/updater"
	"github.com/DataDog/datadog-agent/comp/updater/rc"
	"github.com/DataDog/datadog-agent/pkg/updater"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Module is the fx module for the updater.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newUpdaterComponent),
	)
}

// Options contains the options for the updater.
type Options struct {
	Package string
}

// Params contains the parameters to build the updater.o
type Params struct {
	fx.In

	Log          log.Component
	Config       config.Component
	RemoteConfig rc.Component
	Options      Options
}

func newUpdaterComponent(params Params) (updatercomp.Component, error) {
	updater, err := updater.NewUpdater(params.RemoteConfig, params.Options.Package)
	if err != nil {
		return nil, fmt.Errorf("could not create updater: %w", err)
	}
	return updater, nil
}
