// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package phonehomeimpl wires core-owned PAR startup gating to RC status.
package phonehomeimpl

import (
	"context"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	phonehome "github.com/DataDog/datadog-agent/comp/privateactionrunner/phonehome/def"
	rcservice "github.com/DataDog/datadog-agent/comp/remote-config/rcservice/def"
	engine "github.com/DataDog/datadog-agent/pkg/privateactionrunner/phonehome"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type Requires struct {
	Lifecycle compdef.Lifecycle
	Config    config.Component
	RC        option.Option[rcservice.Component]
}

type Provides struct{ Comp phonehome.Component }
type component struct{}

func NewComponent(r Requires) Provides {
	c := &component{}
	if !engine.Enabled(r.Config) {
		return Provides{Comp: c}
	}
	rc, _ := r.RC.Get()
	rcSource := engine.NewSource(r.Config, rc)
	discovery := engine.NewDiscovery(r.Config, rcSource)
	source := engine.NewScopedSource(rcSource, discovery)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	r.Lifecycle.Append(compdef.Hook{
		OnStart: func(context.Context) error {
			go func() {
				defer close(done)
				discovery.Start()
				defer discovery.Stop()
				engine.Run(ctx, r.Config, source)
			}()
			return nil
		},
		OnStop: func(stop context.Context) error {
			cancel()
			select {
			case <-done:
			case <-stop.Done():
			}
			return nil
		},
	})
	return Provides{Comp: c}
}
