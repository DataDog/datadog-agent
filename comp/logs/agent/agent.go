// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package agent

import (
	"context"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	logComponent "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/logs"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	"go.uber.org/fx"
)

type dependencies struct {
	fx.In

	Lc     fx.Lifecycle
	Log    logComponent.Component
	Config configComponent.Component
}

type logsAgent struct {
	logsAgent *logs.Agent
	log       logComponent.Component
	config    configComponent.Component
}

func newLogsAgent(deps dependencies) Component {
	logsAgent := &logsAgent{log: deps.Log, config: deps.Config}
	deps.Lc.Append(fx.Hook{
		OnStart: logsAgent.start,
		OnStop:  logsAgent.stop,
	})
	return logsAgent
}

func (l *logsAgent) start(context.Context) error {

	if l.config.GetBool("logs_enabled") || l.config.GetBool("log_enabled") {
		if l.config.GetBool("log_enabled") {
			l.log.Warn(`"log_enabled" is deprecated, use "logs_enabled" instead`)
		}
		logsAgent, err := logs.CreateAgent()
		if err != nil {
			l.log.Error("Could not start logs-agent: ", err)
			return err
		}
		l.logsAgent = logsAgent
	} else {
		l.log.Info("logs-agent disabled")
	}

	return nil
}

func (l *logsAgent) stop(context.Context) error {
	if l.logsAgent != nil {
		l.logsAgent.Stop()
	}
	return nil
}

func (l *logsAgent) AddScheduler(scheduler schedulers.Scheduler) {
	if l.logsAgent != nil {
		l.logsAgent.AddScheduler(scheduler)
	}
}

func (l *logsAgent) IsRunning() bool {
	return logs.IsAgentRunning()
}

func (l *logsAgent) GetMessageReceiver() *diagnostic.BufferedMessageReceiver {
	return logs.GetMessageReceiver()
}

func (l *logsAgent) Flush(ctx context.Context) {
	logs.Flush(ctx)
}
