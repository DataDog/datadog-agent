// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package agent

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	"github.com/DataDog/datadog-agent/pkg/util"
	"go.uber.org/fx"
)

type mockLogsAgent struct {
	isRunning       bool
	addedSchedulers []schedulers.Scheduler
	hasFlushed      bool
	flushDelay      time.Duration
}

func newMock(deps dependencies) util.Optional[Mock] {
	logsAgent := &mockLogsAgent{
		hasFlushed:      false,
		addedSchedulers: make([]schedulers.Scheduler, 0),
		isRunning:       false,
		flushDelay:      0,
	}
	deps.Lc.Append(fx.Hook{
		OnStart: logsAgent.start,
		OnStop:  logsAgent.stop,
	})
	return util.NewOptional[Mock](logsAgent)
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

// Serverless methods
func (a *mockLogsAgent) Start() error {
	return a.start(context.TODO())
}

func (a *mockLogsAgent) Stop() {
	_ = a.stop(context.TODO())
}

func (a *mockLogsAgent) Flush(ctx context.Context) {
	select {
	case <-ctx.Done():
		a.hasFlushed = false
	case <-time.NewTimer(a.flushDelay).C:
		a.hasFlushed = true
	}
}

func (a *mockLogsAgent) GetPipelineProvider() pipeline.Provider {
	return nil
}
