// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package localapiimpl implements the installer local api component.
package localapiimpl

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	localapi "github.com/DataDog/datadog-agent/comp/updater/localapi/def"
	updatercomp "github.com/DataDog/datadog-agent/comp/updater/updater"
	"github.com/DataDog/datadog-agent/pkg/fleet/daemon"
)

// Requires defines the dependencies for the updater local api component.
type Requires struct {
	Lifecycle compdef.Lifecycle

	Config  config.Component
	Updater updatercomp.Component
	Log     log.Component
}

// Provides defines the output of the updater local api component.
type Provides struct {
	Comp localapi.Component
}

// NewComponent creates a new updater local api component.
func NewComponent(reqs Requires) (Provides, error) {
	localAPI, err := daemon.NewLocalAPI(reqs.Updater)
	if err != nil {
		return Provides{}, fmt.Errorf("could not create local API: %w", err)
	}
	reqs.Lifecycle.Append(compdef.Hook{OnStart: localAPI.Start, OnStop: localAPI.Stop})
	return Provides{Comp: localAPI}, nil
}
