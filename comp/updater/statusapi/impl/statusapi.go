// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package statusapiimpl implements the installer read-only status api component.
package statusapiimpl

import (
	"context"

	compdef "github.com/DataDog/datadog-agent/comp/def"
	statusapi "github.com/DataDog/datadog-agent/comp/updater/statusapi/def"
	updatercomp "github.com/DataDog/datadog-agent/comp/updater/updater/def"
	"github.com/DataDog/datadog-agent/pkg/fleet/daemon"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

// nopStatusAPI stands in when the listener could not be created, so that the
// lifecycle hook stays unconditional.
type nopStatusAPI struct{}

func (nopStatusAPI) Start(context.Context) error { return nil }
func (nopStatusAPI) Stop(context.Context) error  { return nil }

// NewComponent creates a new installer status api component.
//
// A listener that cannot be created is logged and swallowed rather than returned:
// this listener only feeds host metadata, while the daemon it belongs to applies
// remote upgrades. Failing here would trade a missing metadata payload for a host
// that no longer takes upgrades at all — and the socket lives in a directory the
// Agent user can write to, so an unprivileged process has some influence over
// whether the bind succeeds.
func NewComponent(reqs Requires) Provides {
	api, err := daemon.NewStatusAPI(reqs.Updater)
	if err != nil {
		log.Errorf("Could not create the installer status API, installer metadata will report the installer as unreachable: %v", err)
		api = nopStatusAPI{}
	}
	reqs.Lifecycle.Append(compdef.Hook{OnStart: api.Start, OnStop: api.Stop})
	return Provides{Comp: api}
}
