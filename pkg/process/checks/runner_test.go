// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var (
	runOptionsWithStandard = RunOptions{
		RunStandard: true,
		RunRealtime: false,
	}
	runOptionsWithRealTime = RunOptions{
		RunStandard: false,
		RunRealtime: true,
	}
	runOptionsWithBoth = RunOptions{
		RunStandard: true,
		RunRealtime: true,
	}
)

func TestRunnerWithRealTime(t *testing.T) {
	tests := []struct {
		desc       string
		rtEnabled  bool
		expectRuns []RunOptions
	}{
		{
			desc:      "rt-enabled",
			rtEnabled: true,
			expectRuns: []RunOptions{
				runOptionsWithStandard,
				runOptionsWithBoth,
				runOptionsWithRealTime,
				runOptionsWithRealTime,
				runOptionsWithRealTime,
				runOptionsWithRealTime,
				runOptionsWithBoth,
				runOptionsWithRealTime,
			},
		},
		{
			desc:      "rt-disabled",
			rtEnabled: false,
			expectRuns: []RunOptions{
				runOptionsWithStandard,
				runOptionsWithStandard,
				runOptionsWithStandard,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			exitChan := make(chan struct{})
			rtIntervalChan := make(chan time.Duration)
			defer close(rtIntervalChan)

			var runs []RunOptions
			runCheck := func(options RunOptions) {
				runs = append(runs, options)
			}

			tickerCh := make(chan time.Time)
			defer close(tickerCh)
			ticker := &time.Ticker{
				C: tickerCh,
			}

			runner := &runnerWithRealTime{
				RunnerConfig: RunnerConfig{
					CheckInterval:  10 * time.Second,
					RtInterval:     2 * time.Second,
					ExitChan:       exitChan,
					RtIntervalChan: rtIntervalChan,
					RtEnabled:      func() bool { return test.rtEnabled },
					RunCheck:       runCheck,
				},
				newTicker:  func(time.Duration) *time.Ticker { return ticker },
				stopTicker: func(t *time.Ticker) {},
			}

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				runner.run()
			}()

			for i := 0; i < 7; i++ {
				tickerCh <- time.Now()
			}

			close(exitChan)

			wg.Wait()

			assert.Equal(t, test.expectRuns, runs)
		})
	}
}

func TestRunnerWithRealTime_ReconfigureInterval(t *testing.T) {
	exitChan := make(chan struct{})

	rtIntervalChan := make(chan time.Duration)
	defer close(rtIntervalChan)

	tickerCh := make(chan time.Time)
	defer close(tickerCh)
	ticker := &time.Ticker{
		C: tickerCh,
	}

	r := &runnerWithRealTime{
		RunnerConfig: RunnerConfig{
			CheckInterval: 10 * time.Second,
			RtInterval:    2 * time.Second,

			ExitChan:       exitChan,
			RtIntervalChan: rtIntervalChan,
			RtEnabled:      func() bool { return true },
			RunCheck:       func(options RunOptions) {},
		},
		newTicker:  func(time.Duration) *time.Ticker { return ticker },
		stopTicker: func(t *time.Ticker) {},
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.run()
	}()

	tickerCh <- time.Now()

	rtIntervalChan <- time.Second

	close(exitChan)

	wg.Wait()

	assert.Equal(t, 10, r.ratio)
	assert.Equal(t, 0, r.counter)
}
