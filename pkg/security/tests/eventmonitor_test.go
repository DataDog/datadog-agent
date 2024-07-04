// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor/examples"
	"github.com/avast/retry-go/v4"
	"github.com/stretchr/testify/assert"
)

func TestEventMonitor(t *testing.T) {
	SkipIfNotAvailable(t)

	var sevm *examples.SimpleEventMonitorModule
	test, err := newTestModule(t, nil, nil, withStaticOpts(testOpts{
		disableRuntimeSecurity: true,
		preStartCallback: func(test *testModule) {
			sevm = examples.NewSimpleEventMonitorModule(test.eventMonitor)
			test.eventMonitor.RegisterModule(sevm)
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("fork", func(t *testing.T) {
		forkCount := sevm.Consumer.ForkCount()
		cmd := exec.Command(syscallTester, "fork")
		_ = cmd.Run()

		err := retry.Do(func() error {
			if forkCount+1 <= sevm.Consumer.ForkCount() {
				return nil
			}

			return errors.New("event not received")
		}, retry.Delay(200*time.Millisecond), retry.Attempts(10), retry.DelayType(retry.FixedDelay))
		assert.NoError(t, err)
	})

	t.Run("exec-exit", func(t *testing.T) {
		execCount := sevm.Consumer.ExecCount()
		exitCount := sevm.Consumer.ExitCount()

		lsExecutable := which(t, "ls")
		cmd := exec.Command(lsExecutable, "-l")
		_ = cmd.Run()

		err := retry.Do(func() error {
			if execCount+1 <= sevm.Consumer.ExecCount() && exitCount+1 <= sevm.Consumer.ExitCount() {
				return nil
			}

			return errors.New("event not received")
		}, retry.Delay(200*time.Millisecond), retry.Attempts(10), retry.DelayType(retry.FixedDelay))
		assert.NoError(t, err)
	})
}
