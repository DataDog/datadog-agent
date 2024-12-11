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
	"go.uber.org/atomic"
)

func TestEventMonitor(t *testing.T) {
	SkipIfNotAvailable(t)

	var sec *examples.SimpleEventConsumer
	test, err := newTestModule(t, nil, nil, withStaticOpts(testOpts{
		disableRuntimeSecurity: true,
		preStartCallback: func(test *testModule) {
			sec = examples.NewSimpleEventConsumer(test.eventMonitor)
			test.eventMonitor.RegisterEventConsumer(sec)
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
		forkCount := sec.ForkCount()
		cmd := exec.Command(syscallTester, "fork")
		_ = cmd.Run()

		err := retry.Do(func() error {
			if forkCount+1 <= sec.ForkCount() {
				return nil
			}

			return errors.New("event not received")
		}, retry.Delay(200*time.Millisecond), retry.Attempts(10), retry.DelayType(retry.FixedDelay))
		assert.NoError(t, err)
	})

	t.Run("exec-exit", func(t *testing.T) {
		execCount := sec.ExecCount()
		exitCount := sec.ExitCount()

		lsExecutable := which(t, "ls")
		cmd := exec.Command(lsExecutable, "-l")
		_ = cmd.Run()

		err := retry.Do(func() error {
			if execCount+1 <= sec.ExecCount() && exitCount+1 <= sec.ExitCount() {
				return nil
			}

			return errors.New("event not received")
		}, retry.Delay(200*time.Millisecond), retry.Attempts(10), retry.DelayType(retry.FixedDelay))
		assert.NoError(t, err)
	})
}

func TestEventMonitorNoEnvs(t *testing.T) {
	SkipIfNotAvailable(t)

	var sec *examples.SimpleEventConsumer
	test, err := newTestModule(t, nil, nil, withStaticOpts(testOpts{
		disableRuntimeSecurity:   true,
		disableEnvVarsResolution: true,
		preStartCallback: func(test *testModule) {
			sec = examples.NewSimpleEventConsumer(test.eventMonitor)
			test.eventMonitor.RegisterEventConsumer(sec)
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	lsExecutable := which(t, "ls")

	foundLs := atomic.NewBool(false)

	sec.AddHandler(func(evt *examples.SimpleEvent) {
		if len(evt.Envp) != 0 {
			t.Errorf("unexpected envp: %+v", evt)
		}

		if evt.ExecFilePath == lsExecutable {
			foundLs.Store(true)
		}
	})

	cmd := exec.Command(lsExecutable, "-l")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	err = retry.Do(func() error {
		if foundLs.Load() {
			return nil
		}
		return errors.New("event not received")
	}, retry.Delay(200*time.Millisecond), retry.Attempts(10), retry.DelayType(retry.FixedDelay))
	assert.NoError(t, err)
}
