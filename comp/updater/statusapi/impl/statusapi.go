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

// component owns the read-only status listener. The listener is optional: api is
// nil when it could not be created.
type component struct {
	updater updatercomp.Component
	api     daemon.StatusAPI
}

// NewComponent creates a new installer status api component.
func NewComponent(reqs Requires) Provides {
	c := &component{updater: reqs.Updater}
	reqs.Lifecycle.Append(compdef.Hook{OnStart: c.Start, OnStop: c.Stop})
	return Provides{Comp: c}
}

// Start creates the status listener and serves it.
//
// A failure is logged and swallowed rather than returned: this listener only feeds
// host metadata, while the daemon it belongs to applies remote upgrades. Failing
// construction would trade a missing metadata payload for a host that no longer
// takes upgrades at all — and the socket lives in a directory the Agent user can
// write to, so an unprivileged process has some influence over whether the bind
// succeeds.
func (c *component) Start(ctx context.Context) error {
	api, err := daemon.NewStatusAPI(c.updater)
	if err != nil {
		log.Errorf("Could not start the installer status API, installer metadata will report the installer as unreachable: %v", err)
		return nil
	}
	c.api = api
	return api.Start(ctx)
}

// Stop stops the status listener if it was started.
func (c *component) Stop(ctx context.Context) error {
	if c.api == nil {
		return nil
	}
	return c.api.Stop(ctx)
}
