// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package collectorimpl

import (
	"go.uber.org/fx"

	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	haagent "github.com/DataDog/datadog-agent/comp/haagent/def"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// testDependencies mirrors the dependencies struct but uses fx.In so that
// fxutil.Test can inject it. Use makeDeps to convert to the actual dependencies
// type before calling newCollector or newProvides.
type testDependencies struct {
	fx.In

	Lc             compdef.Lifecycle
	Config         config.Component
	Log            log.Component
	HaAgent        haagent.Component
	HealthPlatform healthplatform.Component
	Hostname       hostnameinterface.Component

	SenderManager    sender.SenderManager
	MetricSerializer option.Option[serializer.MetricSerializer]
	AgentTelemetry   option.Option[agenttelemetry.Component]
}

func makeDeps(td testDependencies) dependencies {
	return dependencies{
		Lc:               td.Lc,
		Config:           td.Config,
		Log:              td.Log,
		HaAgent:          td.HaAgent,
		HealthPlatform:   td.HealthPlatform,
		Hostname:         td.Hostname,
		SenderManager:    td.SenderManager,
		MetricSerializer: td.MetricSerializer,
		AgentTelemetry:   td.AgentTelemetry,
	}
}
