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
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

type mockLogsAgent struct {
	isRunning       bool
	addedSchedulers []schedulers.Scheduler
	hasFlushed      bool
	flushDelay      time.Duration
	logSources      *sources.LogSources
}

func newMock(deps dependencies) optional.Option[Mock] {
	panic("not called")
}

func (a *mockLogsAgent) start(context.Context) error {
	panic("not called")
}

func (a *mockLogsAgent) stop(context.Context) error {
	panic("not called")
}

func (a *mockLogsAgent) AddScheduler(scheduler schedulers.Scheduler) {
	panic("not called")
}

func (a *mockLogsAgent) SetSources(sources *sources.LogSources) {
	panic("not called")
}

func (a *mockLogsAgent) IsRunning() bool {
	panic("not called")
}

func (a *mockLogsAgent) GetMessageReceiver() *diagnostic.BufferedMessageReceiver {
	panic("not called")
}

func (a *mockLogsAgent) GetSources() *sources.LogSources {
	panic("not called")
}

// Serverless methods
func (a *mockLogsAgent) Start() error {
	panic("not called")
}

func (a *mockLogsAgent) Stop() {
	panic("not called")
}

func (a *mockLogsAgent) Flush(ctx context.Context) {
	panic("not called")
}

func (a *mockLogsAgent) GetPipelineProvider() pipeline.Provider {
	panic("not called")
}
