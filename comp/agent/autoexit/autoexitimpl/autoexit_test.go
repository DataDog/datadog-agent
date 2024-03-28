// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package autoexitimpl

import (
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/agent/autoexit"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// This test reuse some part of https://go.dev/src/os/signal/signal_test.go

// settleTime is an upper bound on how long we expect signals to take to be
// delivered. Lower values make the test faster, but also flakier — especially
// on heavily loaded systems.
//
// The current value is set based on flakes observed in the Go builders.
var settleTime = 100 * time.Millisecond

// fatalWaitingTime is an absurdly long time to wait for signals to be
// delivered but, using it, we (hopefully) eliminate test flakes on the
// build servers. See #46736 for discussion.
var fatalWaitingTime = 10 * time.Second

func TestAutoexitInterrupt(t *testing.T) {
	c := make(chan os.Signal, 1)

	// redirect emitted os.Interrupt signals to c channel
	signal.Notify(c, os.Interrupt)

	var _ = setupAutoExit(t, map[string]interface{}{
		"auto_exit.noprocess.enabled": true,
		"auto_exit.validation_period": 5,
		// this param will match with every process, and will force autoexit emit Interrupt signal
		"auto_exit.noprocess.excluded_processes": ".*",
	})

	waitSig(t, c, os.Interrupt)
}

func setupAutoExit(t *testing.T, p map[string]interface{}) autoexit.Component {
	return fxutil.Test[autoexit.Component](t, fx.Options(
		fx.Supply(params{defaultExitTicker: 1 * time.Second}),
		fx.Provide(newAutoExit),
		logimpl.MockModule(),
		config.MockModule(),
		fx.Replace(config.MockParams{Overrides: p}),
	))
}

func waitSig(t *testing.T, c <-chan os.Signal, sig os.Signal) {
	t.Helper()

	// Sleep multiple times to give the kernel more tries to
	// deliver the signal.
	start := time.Now()
	timer := time.NewTimer(settleTime / 10)
	defer timer.Stop()
	// If the caller notified for all signals on c, filter out SIGURG,
	// which is used for runtime preemption and can come at unpredictable times.
	// General user code should filter out all unexpected signals instead of just
	// SIGURG, but since os/signal is tightly coupled to the runtime it seems
	// appropriate to be stricter here.
	for time.Since(start) < fatalWaitingTime {
		select {
		case s := <-c:
			if s == sig {
				return
			}
			if s != syscall.SIGURG {
				t.Fatalf("signal was %v, want %v", s, sig)
			}
		case <-timer.C:
			timer.Reset(settleTime / 10)
		}
	}
	t.Fatalf("timeout after %v waiting for %v", fatalWaitingTime, sig)
}
