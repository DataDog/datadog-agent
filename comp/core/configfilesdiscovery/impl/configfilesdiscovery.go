// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package configfilesdiscoveryimpl implements the configfilesdiscovery component.
package configfilesdiscoveryimpl

import (
	"context"
	"time"

	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/comp/core/config"
	configfilesdiscovery "github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/def"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	heartbeatIntervalConfigKey = "config_files_discovery.heartbeat_interval"
	heartbeatJitterConfigKey   = "config_files_discovery.heartbeat_jitter"
	startupJitterConfigKey     = "config_files_discovery.startup_jitter"
)

// Requires defines the dependencies for the config files discovery component.
type Requires struct {
	compdef.In

	Lifecycle     compdef.Lifecycle
	Config        config.Component
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
	return newComponentWithSchedulerConfig(ad, resolver, sender, configCollectors, defaultADSchedulerConfig())
}

func newComponentWithSchedulerConfig(
	ad autodiscovery.Component,
	resolver targetResolver,
	sender collectedConfigSender,
	configCollectors map[string]ConfigCollector,
	schedulerCfg adSchedulerConfig,
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
		scheduler: newADSchedulerWithConfig(resolver, readers, configCollectors, sender, schedulerCfg),
	}
}

// NewComponent creates the config files discovery component.
func NewComponent(reqs Requires) Provides {
	schedulerCfg := defaultADSchedulerConfig()
	if reqs.Config != nil {
		schedulerCfg = adSchedulerConfigFromAgentConfig(reqs.Config)
	}

	c := newComponentWithSchedulerConfig(
		reqs.Autodiscovery,
		targetResolver{store: reqs.WorkloadMeta},
		newEventPlatformCollectedConfigSender(reqs.EventPlatform, reqs.Hostname.GetSafe(context.Background())),
		reqs.Collectors,
		schedulerCfg,
	)
	reqs.Lifecycle.Append(compdef.Hook{OnStart: c.start, OnStop: c.stop})
	return Provides{Comp: c}
}

func adSchedulerConfigFromAgentConfig(agentConfig config.Component) adSchedulerConfig {
	cfg := defaultADSchedulerConfig()
	if agentConfig == nil {
		return cfg
	}

	heartbeatInterval := agentConfig.GetDuration(heartbeatIntervalConfigKey)
	if heartbeatInterval <= 0 {
		log.Warnf("configured %s must be positive, using default %s", heartbeatIntervalConfigKey, defaultHeartbeatInterval)
	} else {
		cfg.heartbeatInterval = heartbeatInterval
	}

	heartbeatJitter := agentConfig.GetDuration(heartbeatJitterConfigKey)
	jitterLimit := heartbeatJitterLimit(cfg.heartbeatInterval)
	switch {
	case heartbeatJitter < 0:
		log.Warnf("configured %s must be non-negative, using 0", heartbeatJitterConfigKey)
		cfg.heartbeatJitter = 0
	case heartbeatJitter > jitterLimit:
		log.Warnf("configured %s exceeds maximum %s for heartbeat interval %s, clamping", heartbeatJitterConfigKey, jitterLimit, cfg.heartbeatInterval)
		cfg.heartbeatJitter = jitterLimit
	default:
		cfg.heartbeatJitter = heartbeatJitter
	}

	startupJitter := agentConfig.GetDuration(startupJitterConfigKey)
	if startupJitter < 0 {
		log.Warnf("configured %s must be non-negative, using 0", startupJitterConfigKey)
		cfg.startupJitter = 0
	} else {
		cfg.startupJitter = startupJitter
	}

	if cfg.heartbeatCheckInterval > cfg.heartbeatInterval/10 {
		cfg.heartbeatCheckInterval = cfg.heartbeatInterval / 10
	}
	if cfg.heartbeatCheckInterval <= 0 {
		cfg.heartbeatCheckInterval = time.Second
	}
	return cfg
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
