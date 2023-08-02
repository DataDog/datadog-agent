// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package agent

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	"github.com/DataDog/datadog-agent/pkg/util"
	"go.uber.org/fx"
)

type mockLogsAgent struct {
	isRunning       bool
	addedSchedulers []schedulers.Scheduler
}

func newMock(deps dependencies) util.Optional[Component] {
	logsAgent := &mockLogsAgent{}
	deps.Lc.Append(fx.Hook{
		OnStart: logsAgent.start,
		OnStop:  logsAgent.stop,
	})
	return util.NewOptional[Component](logsAgent)
}

func (a *mockLogsAgent) start(context.Context) error {
	a.isRunning = true
	return nil
}

func (a *mockLogsAgent) stop(context.Context) error {
	a.isRunning = false
	return nil
}

func (a *mockLogsAgent) AddScheduler(scheduler schedulers.Scheduler) {
	a.addedSchedulers = append(a.addedSchedulers, scheduler)
}

func (a *mockLogsAgent) IsRunning() bool {
	return a.isRunning
}

func (a *mockLogsAgent) GetMessageReceiver() *diagnostic.BufferedMessageReceiver {
	return nil
}

func (a *mockLogsAgent) Flush(ctx context.Context) {}

func (a *mockLogsAgent) GetPipelineProvider() pipeline.Provider {
	return nil
}
