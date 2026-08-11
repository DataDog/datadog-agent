// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package statusapiimpl implements the installer read-only status api component.
package statusapiimpl

import (
	"fmt"

	compdef "github.com/DataDog/datadog-agent/comp/def"
	statusapi "github.com/DataDog/datadog-agent/comp/updater/statusapi/def"
	updatercomp "github.com/DataDog/datadog-agent/comp/updater/updater/def"
	"github.com/DataDog/datadog-agent/pkg/fleet/daemon"
)

// Requires defines the dependencies for the installer status api component.
type Requires struct {
	Lifecycle compdef.Lifecycle

	Updater updatercomp.Component
}

// Provides defines the output of the installer status api component.
type Provides struct {
	Comp statusapi.Component
}

// NewComponent creates a new installer status api component.
func NewComponent(reqs Requires) (Provides, error) {
	statusAPI, err := daemon.NewStatusAPI(reqs.Updater)
	if err != nil {
		return Provides{}, fmt.Errorf("could not create status API: %w", err)
	}
	reqs.Lifecycle.Append(compdef.Hook{OnStart: statusAPI.Start, OnStop: statusAPI.Stop})
	return Provides{Comp: statusAPI}, nil
}
