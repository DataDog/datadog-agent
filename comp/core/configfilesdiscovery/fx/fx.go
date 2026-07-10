// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package fx provides fx wiring for the config files discovery component.
package fx

import (
	"go.uber.org/fx"

	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	configfilesdiscovery "github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/def"
	configfilesdiscoveryimpl "github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/impl"
	"github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/impl/collectors"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(newOptionalComponent),
		fx.Invoke(func(_ option.Option[configfilesdiscovery.Component]) {}),
	)
}

// Requires defines the dependencies needed to decide whether to start config
// files discovery.
type Requires struct {
	compdef.In

	Lifecycle     compdef.Lifecycle
	Config        config.Component
	Autodiscovery autodiscovery.Component
	Hostname      hostname.Component
	WorkloadMeta  workloadmeta.Component
	EventPlatform eventplatform.Component
}

// Provides defines the optional output of the config files discovery fx module.
type Provides struct {
	compdef.Out

	Comp option.Option[configfilesdiscovery.Component]
}

func newOptionalComponent(reqs Requires) Provides {
	if !reqs.Config.GetBool("config_files_discovery.enabled") {
		return Provides{Comp: option.None[configfilesdiscovery.Component]()}
	}

	provides := configfilesdiscoveryimpl.NewComponent(configfilesdiscoveryimpl.Requires{
		Lifecycle:     reqs.Lifecycle,
		Autodiscovery: reqs.Autodiscovery,
		Hostname:      reqs.Hostname,
		WorkloadMeta:  reqs.WorkloadMeta,
		EventPlatform: reqs.EventPlatform,
		Collectors:    defaultCollectors(),
	})
	return Provides{Comp: option.New(provides.Comp)}
}

func defaultCollectors() map[string]configfilesdiscoveryimpl.ConfigCollector {
	return map[string]configfilesdiscoveryimpl.ConfigCollector{
		collectors.KafkaIntegrationName: collectors.NewKafka(),
		collectors.RedisIntegrationName: collectors.NewRedis(),
	}
}
