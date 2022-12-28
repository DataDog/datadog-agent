// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	legacyConfig "github.com/DataDog/datadog-agent/pkg/process/config"

	"go.uber.org/fx"
)

type dependencies struct {
	fx.In

	cfg               config.Component
	legacyAgentConfig *legacyConfig.AgentConfig
	enabledChecks     []checks.Check
}

var _ Component = &runner{}

type runner struct {
	cfg               config.Component
	legacyAgentConfig *legacyConfig.AgentConfig
	enabledChecks     []checks.Check

	rtIntervalCh chan time.Duration

	stopper chan struct{}
}

func newRunner(deps dependencies) Component {
	return &runner{
		cfg:               deps.cfg,
		legacyAgentConfig: deps.legacyAgentConfig,
		enabledChecks:     deps.enabledChecks,

		stopper: make(chan struct{}),
	}
}

func (r *runner) run() error {
	var wg sync.WaitGroup
	defer wg.Wait()

	for _, c := range r.enabledChecks {
		runFunc, err := r.runnerForCheck(c)
		if err != nil {
			return fmt.Errorf("error starting check %s: %s", c.Name(), err)
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			runFunc()
		}()
	}

	<-r.stopper
	wg.Wait()

	return nil
}

func (r *runner) stop() error {
	close(r.stopper)
	return nil
}

func (r *runner) runnerForCheck(c checks.Check) (func(), error) {
	withRealTime, ok := c.(checks.CheckWithRealTime)
	if !r.cfg.GetBool("process_config.disable_realtime_checks") || !ok {
		return r.basicRunner(c, results, exit), nil
	}

	return checks.NewRunnerWithRealTime(
		checks.RunnerConfig{
			CheckInterval: r.legacyAgentConfig.CheckInterval(withRealTime.Name()),
			RtInterval:    r.legacyAgentConfig.CheckInterval(withRealTime.RealTimeName()),

			ExitChan:       r.stopper,
			RtIntervalChan: r.rtIntervalCh,
			RtEnabled: func() bool {
				return r.realTimeEnabled.Load()
			},
			RunCheck: func(options checks.RunOptions) {
				l.runCheckWithRealTime(withRealTime, results, rtResults, options)
			},
		},
	)
}
