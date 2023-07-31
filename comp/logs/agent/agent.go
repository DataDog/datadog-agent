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
	"github.com/DataDog/datadog-agent/pkg/util"
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

func newLogsAgent(deps dependencies) util.Optional[Component] {

	if deps.Config.GetBool("logs_enabled") || deps.Config.GetBool("log_enabled") {
		if deps.Config.GetBool("log_enabled") {
			deps.Log.Warn(`"log_enabled" is deprecated, use "logs_enabled" instead`)
		}

		logsAgent := &logsAgent{log: deps.Log, config: deps.Config}
		deps.Lc.Append(fx.Hook{
			OnStart: logsAgent.start,
			OnStop:  logsAgent.stop,
		})

		return util.NewOptional[Component](logsAgent)
	}

	deps.Log.Info("logs-agent disabled")
	return util.NewNoneOptional[Component]()

}

func (l *logsAgent) start(context.Context) error {

	logsAgent, err := logs.CreateAgent()
	if err != nil {
		l.log.Error("Could not start logs-agent: ", err)
		return err
	}
	l.logsAgent = logsAgent
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
