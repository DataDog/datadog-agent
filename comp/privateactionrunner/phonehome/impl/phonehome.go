// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package phonehomeimpl wires core-owned PAR launch/reconciliation to RC status.
package phonehomeimpl

import (
	"context"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	phonehome "github.com/DataDog/datadog-agent/comp/privateactionrunner/phonehome/def"
	rcservice "github.com/DataDog/datadog-agent/comp/remote-config/rcservice/def"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	engine "github.com/DataDog/datadog-agent/pkg/privateactionrunner/phonehome"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Requires are core dependencies; missing RC remains an explicit waiting state.
type Requires struct {
	Lifecycle compdef.Lifecycle
	Config    config.Component
	Hostname  hostname.Component
	Log       log.Component
	RC        option.Option[rcservice.Component]
}

// Provides registers the core phone-home lifecycle without owning Go enrollment.
type Provides struct{ Comp phonehome.Component }

type component struct{}

// NewComponent installs a cancellable background controller, not a startup wait.
func NewComponent(r Requires) Provides {
	c := &component{}
	if !enrollment.PhoneHomePOC() || !r.Config.GetBool(par.Enabled) {
		return Provides{Comp: c}
	}
	rc, _ := r.RC.Get()
	source := engine.NewSource(r.Config, rc, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	r.Lifecycle.Append(compdef.Hook{
		OnStart: func(context.Context) error {
			go func() {
				defer close(done)
				defer source.Demand(false)
				host, err := r.Hostname.Get(ctx)
				if err != nil {
					r.Log.Warn("PAR phone-home could not resolve hostname")
					return
				}
				engine.Run(ctx, r.Config, host, source)
			}()
			return nil
		},
		OnStop: func(stop context.Context) error {
			cancel()
			source.Demand(false)
			select {
			case <-done:
			case <-stop.Done():
			}
			return nil
		},
	})
	return Provides{Comp: c}
}
