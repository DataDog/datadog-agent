// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"errors"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RunnerConfig implements config for runners that work with CheckWithRealTime
type RunnerConfig struct {
	CheckInterval time.Duration
	RtInterval    time.Duration

	ExitChan       chan struct{}
	RtIntervalChan chan time.Duration
	RtEnabled      func() bool
	RunCheck       func(options RunOptions)
}

type runnerWithRealTime struct {
	RunnerConfig
	ratio      int
	counter    int
	newTicker  func(d time.Duration) *time.Ticker
	stopTicker func(t *time.Ticker)
}

// NewRunnerWithRealTime creates a runner func for CheckWithRealTime
func NewRunnerWithRealTime(config RunnerConfig) (func(), error) {
	_, err := getRtRatio(config.CheckInterval, config.RtInterval)
	if err != nil {
		return nil, err
	}
	r := &runnerWithRealTime{
		RunnerConfig: config,
		newTicker:    time.NewTicker,
		stopTicker: func(t *time.Ticker) {
			t.Stop()
		},
	}
	return r.run, nil
}

// run performs runs for CheckWithRealTime checks
func (r *runnerWithRealTime) run() {
	var err error
	r.ratio, err = getRtRatio(r.CheckInterval, r.RtInterval)
	if err != nil {
		return
	}

	// Run the check the first time to prime the caches.
	r.RunCheck(RunOptions{
		RunStandard: true,
	})

	ticker := r.newTicker(r.RtInterval)
	for {
		select {
		case <-ticker.C:
			if r.counter == r.ratio {
				r.counter = 0
			}

			rtEnabled := r.RtEnabled()
			if rtEnabled || r.counter == 0 {
				r.RunCheck(RunOptions{
					RunStandard: r.counter == 0,
					RunRealtime: rtEnabled,
				})
			}

			r.counter++
		case d := <-r.RtIntervalChan:
			// Live-update the ticker.
			newRatio, err := getRtRatio(r.CheckInterval, d)
			if err != nil {
				log.Errorf("failed to apply new RT interval: %v", err)
				continue
			}
			r.RtInterval = d
			r.stopTicker(ticker)
			ticker = r.newTicker(d)

			r.ratio = newRatio
			r.counter = 0
		case _, ok := <-r.ExitChan:
			if !ok {
				return
			}
		}
	}
}

func getRtRatio(checkInterval, rtInterval time.Duration) (int, error) {
	if checkInterval < rtInterval {
		return -1, errors.New("check interval should be larger or equal to RT interval")
	}
	if checkInterval%rtInterval != 0 {
		return -1, errors.New("check interval should be divisible by RT interval")
	}
	return int(checkInterval / rtInterval), nil
}
