// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package configfilesdiscoveryimpl implements the configfilesdiscovery component.
package configfilesdiscoveryimpl

import (
	"context"

	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	configfilesdiscovery "github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/def"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
)

// Requires defines the dependencies for the config files discovery component.
type Requires struct {
	compdef.In

	Lifecycle     compdef.Lifecycle
	Autodiscovery autodiscovery.Component
	Hostname      hostname.Component
	WorkloadMeta  workloadmeta.Component
	EventPlatform eventplatform.Component
	Collectors    map[string]ConfigCollector
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
	sender collectedConfigSender,
	configCollectors map[string]ConfigCollector,
) *component {
	readers := map[RuntimeType]configReaderFactory{
		RuntimeDocker:     newDockerConfigReader,
		RuntimeKubernetes: newKubernetesConfigReader,
	}
	if configCollectors == nil {
		configCollectors = map[string]ConfigCollector{}
	}
	return &component{
		ad:        ad,
		scheduler: newADScheduler(resolver, readers, configCollectors, sender),
	}
}

// NewComponent creates the config files discovery component.
func NewComponent(reqs Requires) Provides {
	c := newComponent(
		reqs.Autodiscovery,
		targetResolver{store: reqs.WorkloadMeta},
		newEventPlatformCollectedConfigSender(reqs.EventPlatform, reqs.Hostname.GetSafe(context.Background())),
		reqs.Collectors,
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
