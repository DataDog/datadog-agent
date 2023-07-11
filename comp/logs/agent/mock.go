package agent

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
	"go.uber.org/fx"
)

type mockLogsAgent struct {
	isRunning       bool
	addedSchedulers []*autodiscovery.AutoConfig
}

func newMock(deps dependencies) Component {
	logsAgent := &mockLogsAgent{}
	deps.Lc.Append(fx.Hook{
		OnStart: logsAgent.start,
		OnStop:  logsAgent.stop,
	})
	return logsAgent
}

func (a *mockLogsAgent) start(context.Context) error {
	a.isRunning = true
	return nil
}

func (a *mockLogsAgent) stop(context.Context) error {
	a.isRunning = false
	return nil
}

func (a *mockLogsAgent) AddScheduler(ac *autodiscovery.AutoConfig) {
	a.addedSchedulers = append(a.addedSchedulers, ac)
}

func (a *mockLogsAgent) IsRunning() bool {
	return a.isRunning
}
