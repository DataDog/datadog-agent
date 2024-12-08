// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && test

package monitor

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor/consumers/testutil"
)

// InitializeEventConsumer initializes the event consumer for testing
func InitializeEventConsumer(t *testing.T) {
	// This is a placeholder for the actual implementation
	consumer := testutil.NewTestProcessConsumer(t)
	consumer.SubscribeExec(func(pid uint32) {
		processMonitor.tel.events.Add(1)
		processMonitor.tel.exec.Add(1)
		if processMonitor.hasExecCallbacks.Load() {
			processMonitor.handleProcessExec(pid)
		}
	})
	consumer.SubscribeExit(func(pid uint32) {
		processMonitor.tel.events.Add(1)
		processMonitor.tel.exit.Add(1)
		if processMonitor.hasExitCallbacks.Load() {
			processMonitor.handleProcessExit(pid)
		}
	})
}
