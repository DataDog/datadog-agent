// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package datasecurityimpl implements the data security component.
package datasecurityimpl

import (
	"context"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	datasecurity "github.com/DataDog/datadog-agent/comp/datasecurity/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	rcclient "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/def"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// Requires defines the dependencies of the data security component.
type Requires struct {
	Lc       compdef.Lifecycle
	Log      log.Component
	RcClient rcclient.Component
}

// Provides defines the output of the data security component.
type Provides struct {
	Comp datasecurity.Component
}

// component implements the data security component.
type component struct {
	log      log.Component
	rcclient rcclient.Component
}

// NewComponent creates a new data security component.
func NewComponent(reqs Requires) (Provides, error) {
	c := &component{
		log:      reqs.Log,
		rcclient: reqs.RcClient,
	}

	reqs.Lc.Append(compdef.Hook{
		OnStart: c.start,
	})

	return Provides{Comp: c}, nil
}

// start subscribes to the DEBUG remote-config product. The callback runs
// whenever the RC client refreshes the product's targets, including when the
// initial set is empty: that's expected and we acknowledge each entry so the
// backend can reflect the apply state.
func (c *component) start(_ context.Context) error {
	c.rcclient.Subscribe(data.ProductDebug, c.onUpdate)
	c.log.Infof("datasecurity: subscribed to RC product %q", data.ProductDebug)
	return nil
}

// onUpdate is invoked by the RC client with the full set of active configs
// for the DEBUG product. The map is keyed by config path. We log each one
// and report ACKNOWLEDGED back via applyStatus; this scaffolding never fails
// to apply, since "logging" can't really fail.
func (c *component) onUpdate(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) {
	if len(updates) == 0 {
		c.log.Debug("datasecurity: received empty RC update for DEBUG product")
		return
	}

	c.log.Infof("datasecurity: received %d DEBUG RC config(s)", len(updates))
	for path, cfg := range updates {
		c.log.Infof(
			"datasecurity: DEBUG config path=%s version=%d length=%d bytes",
			path, cfg.Metadata.Version, len(cfg.Config),
		)
		c.log.Infof("datasecurity: DEBUG config path=%s payload=%s", path, string(cfg.Config))

		applyStatus(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}
}
