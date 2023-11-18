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

	"github.com/DataDog/datadog-agent/pkg/serverless/daemon"
)

func TestDaemonStopOnSIGINT(t *testing.T) {
	stopCh := make(chan struct{})
	signalCh := make(chan os.Signal, 1)

	serverlessDaemon := daemon.StartDaemon("http://localhost:8124")
	go handleTerminationSignals(serverlessDaemon, stopCh, signalCh)

	signalCh <- syscall.SIGINT

	// Use t.Run with a timeout to allow the goroutine to finish
	t.Run("WaitForStop", func(t *testing.T) {
		select {
		case <-stopCh: // Expected behavior, the daemon should be stopped
		case <-time.After(1000 * time.Millisecond):
			t.Error("Timeout waiting for daemon to stop")
		}
	})
}

func TestDaemonStopOnSIGTERM(t *testing.T) {
	stopCh := make(chan struct{})
	signalCh := make(chan os.Signal, 1)

	serverlessDaemon := daemon.StartDaemon("http://localhost:8124")
	go handleTerminationSignals(serverlessDaemon, stopCh, signalCh)

	signalCh <- syscall.SIGTERM

	// Use t.Run with a timeout to allow the goroutine to finish
	t.Run("WaitForStop", func(t *testing.T) {
		select {
		case <-stopCh: // Expected behavior, the daemon should be stopped
		case <-time.After(1000 * time.Millisecond):
			t.Error("Timeout waiting for daemon to stop")
		}
	})
}
