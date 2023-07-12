// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package agent

import (
	"context"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	logComponent "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/logs"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	adScheduler "github.com/DataDog/datadog-agent/pkg/logs/schedulers/ad"
	"go.uber.org/fx"
)

type dependencies struct {
	fx.In

	Lc     fx.Lifecycle
	Log    logComponent.Component
	Config configComponent.Component
}

type agent struct {
	logsAgent *logs.Agent
	log       logComponent.Component
	config    configComponent.Component
}

func newLogsAgent(deps dependencies) Component {
	logsAgent := &agent{log: deps.Log, config: deps.Config}
	deps.Lc.Append(fx.Hook{
		OnStart: logsAgent.start,
		OnStop:  logsAgent.stop,
	})
	return logsAgent
}

func (a *agent) start(context.Context) error {

	if a.config.GetBool("logs_enabled") || a.config.GetBool("log_enabled") {
		if a.config.GetBool("log_enabled") {
			a.log.Warn(`"log_enabled" is deprecated, use "logs_enabled" instead`)
		}
		logsAgent, err := logs.CreateAgent()
		if err != nil {
			a.log.Error("Could not start logs-agent: ", err)
			return err
		}
		a.logsAgent = logsAgent
	} else {
		a.log.Info("logs-agent disabled")
	}

	return nil
}

func (a *agent) stop(context.Context) error {
	if a.logsAgent != nil {
		a.logsAgent.Stop()
	}
	return nil
}

func (a *agent) AddScheduler(ac *autodiscovery.AutoConfig) {
	if a.logsAgent != nil {
		a.logsAgent.AddScheduler(adScheduler.New(ac))
	}
}

func (a *agent) IsRunning() bool {
	return logs.IsAgentRunning()
}

func (a *agent) GetMessageReceiver() *diagnostic.BufferedMessageReceiver {
	return logs.GetMessageReceiver()
}
