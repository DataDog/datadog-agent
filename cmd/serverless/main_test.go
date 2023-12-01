// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/serverless/daemon"
	"github.com/DataDog/datadog-agent/comp/serverless/daemon/daemonimpl"
	"github.com/DataDog/datadog-agent/pkg/serverless/registration"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestDaemonStopOnTerminationSignals(t *testing.T) {
	stopCh := make(chan struct{})
	serverlessDaemon := fxutil.Test[daemon.Mock](t, fx.Supply(daemonimpl.Params{Addr: "http://localhost:8124", SketchesBucketOffset: time.Second * 10}), daemonimpl.MockModule)
	serverlessDaemon.Start(time.Now(), "/var/task/datadog.yaml", registration.ID("1"), registration.FunctionARN("arn:1"))

	testCases := []struct {
		name   string
		signal syscall.Signal
	}{
		{
			name:   "WaitForStopOnSIGINT",
			signal: syscall.SIGINT,
		},
		{
			name:   "WaitForStopOnSIGTERM",
			signal: syscall.SIGTERM,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			go handleTerminationSignals(serverlessDaemon, stopCh, func(c chan<- os.Signal, sig ...os.Signal) {
				c <- tc.signal
			})

			select {
			// Expected behavior, the daemon should be stopped
			case <-stopCh:
				assert.Equal(t, true, serverlessDaemon.GetStopped())
			case <-time.After(1000 * time.Millisecond):
				t.Error("Timeout waiting for daemon to stop")
			}
		})
	}
}
