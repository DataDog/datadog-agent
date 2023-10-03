// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package pipeline

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	"go.uber.org/fx"
)

type mockLogsAgent struct {
	isRunning  bool
	hasFlushed bool
	flushDelay time.Duration
}

func newMock(deps dependencies) optional.Optional[Mock] {
	logsAgent := &mockLogsAgent{
		hasFlushed: false,
		isRunning:  false,
		flushDelay: 0,
	}
	deps.Lc.Append(fx.Hook{
		OnStart: logsAgent.start,
		OnStop:  logsAgent.stop,
	})
	return optional.NewOptional[Mock](logsAgent)
}

func (a *mockLogsAgent) start(context.Context) error {
	a.isRunning = true
	return nil
}

func (a *mockLogsAgent) stop(context.Context) error {
	a.isRunning = false
	return nil
}

func (a *mockLogsAgent) IsRunning() bool {
	return a.isRunning
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
