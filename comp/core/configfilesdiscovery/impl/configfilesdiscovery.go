// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package configfilesdiscoveryimpl implements the configfilesdiscovery component.
package configfilesdiscoveryimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	configfilesdiscovery "github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

// Requires defines the dependencies for the config files discovery component.
type Requires struct {
	compdef.In

	Lifecycle     compdef.Lifecycle
	Autodiscovery autodiscovery.Component
	WorkloadMeta  workloadmeta.Component
}

// Provides defines the output of the config files discovery component.
type Provides struct {
	compdef.Out

	Comp configfilesdiscovery.Component
}

type component struct {
	ad        autodiscovery.Component
	scheduler scheduler.Scheduler
}

func newComponent(
	ad autodiscovery.Component,
	resolver targetResolver,
) *component {
	return &component{
		ad:        ad,
		scheduler: newADScheduler(resolver),
	}
}

// NewComponent creates the config files discovery component.
func NewComponent(reqs Requires) Provides {
	c := newComponent(
		reqs.Autodiscovery,
		targetResolver{store: reqs.WorkloadMeta},
	)
	reqs.Lifecycle.Append(compdef.Hook{OnStart: c.start, OnStop: c.stop})
	return Provides{Comp: c}
}

func (c *component) start(context.Context) error {
	c.ad.AddScheduler(schedulerName, c.scheduler, true)
	return nil
}

func (c *component) stop(context.Context) error {
	c.ad.RemoveScheduler(schedulerName)
	c.scheduler.Stop()
	return nil
}
