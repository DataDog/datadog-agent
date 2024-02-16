// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package localapiimpl implements the updater local api component.
package localapiimpl

import (
	"fmt"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/updater/localapi"
	updatercomp "github.com/DataDog/datadog-agent/comp/updater/updater"
	"github.com/DataDog/datadog-agent/pkg/updater"
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

	Updater updatercomp.Component
	Log     log.Component
}

func newLocalAPIComponent(dependencies dependencies) (localapi.Component, error) {
	localAPI, err := updater.NewLocalAPI(dependencies.Updater)
	if err != nil {
		return nil, fmt.Errorf("could not create local API: %w", err)
	}
	return localAPI, nil
}
