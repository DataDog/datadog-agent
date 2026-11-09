// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package phonehomeimpl wires core-owned PAR startup gating to RC status.
package phonehomeimpl

import (
	"context"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	phonehome "github.com/DataDog/datadog-agent/comp/privateactionrunner/phonehome/def"
	rcservice "github.com/DataDog/datadog-agent/comp/remote-config/rcservice/def"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	engine "github.com/DataDog/datadog-agent/pkg/privateactionrunner/phonehome"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type Requires struct {
	Lifecycle compdef.Lifecycle
	Config    config.Component
	RC        option.Option[rcservice.Component]
	Telemetry telemetry.Component
}

type Provides struct{ Comp phonehome.Component }
type component struct{ cfg parconfig.RuntimeConfig }

func (c *component) Status() *phonehome.State {
	if !engine.Enabled(c.cfg) {
		return nil
	}
	return engine.Status()
}

func NewComponent(r Requires) Provides {
	cfg := parconfig.New(r.Config)
	c := &component{cfg: cfg}
	if !engine.Enabled(cfg) {
		return Provides{Comp: c}
	}
	rc, _ := r.RC.Get()
	rcSource := engine.NewSource(cfg, rc)
	discovery := engine.NewDiscovery(cfg, rcSource)
	source := engine.NewScopedSource(rcSource, discovery)
	report := newTelemetryReporter(r.Telemetry)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	r.Lifecycle.Append(compdef.Hook{
		OnStart: func(context.Context) error {
			go func() {
				defer close(done)
				discovery.Start()
				defer discovery.Stop()
				engine.Run(ctx, cfg, source, report)
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
