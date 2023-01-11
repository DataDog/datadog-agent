// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests
// +build functionaltests

package tests

import (
	"os/exec"
	"testing"
	"time"

	pmodel "github.com/DataDog/datadog-agent/pkg/process/events/model"
	"github.com/stretchr/testify/assert"
)

func TestProcessMonitoringDisabled(t *testing.T) {
	test, err := newTestModule(t, nil, nil, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("no-event", func(t *testing.T) {
		lsExecutable := which(t, "ls")

		err = test.GetProcessEvent(func() error {
			cmd := exec.Command(lsExecutable)
			return cmd.Run()
		}, func(event *pmodel.ProcessEvent) bool {
			t.Fatal("shouldn't get a process event")
			return true
		}, 3*time.Second, "")
		if err == nil {
			t.Fatal("shouldn't get a process event")
		} else if otherErr, ok := err.(ErrTimeout); !ok {
			t.Fatal(otherErr)
		}
	})
}

func TestProcessMonitoring(t *testing.T) {
	test, err := newTestModule(t, nil, nil, testOpts{
		disableRuntimeSecurity: true,
		enableEventMonitoring:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("fork", func(t *testing.T) {
		err = test.GetProcessEvent(func() error {
			cmd := exec.Command(syscallTester, "fork")
			return cmd.Run()
		}, func(event *pmodel.ProcessEvent) bool {
			return assert.Equal(t, pmodel.Fork.String(), event.EventType.String(), "wrong process event type") &&
				assert.Equal(t, syscallTester, event.Exe, "wrong executable path") &&
				assert.Equal(t, []string{syscallTester, "fork"}, event.Cmdline, "wrong command line") &&
				assertNearTimeObject(t, event.ForkTime)
		}, getEventTimeout, syscallTester, pmodel.Fork)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("exec", func(t *testing.T) {
		lsExecutable := which(t, "ls")
		err = test.GetProcessEvent(func() error {
			cmd := exec.Command(lsExecutable, "-l")
			return cmd.Run()
		}, func(event *pmodel.ProcessEvent) bool {
			return assert.Equal(t, pmodel.Exec.String(), event.EventType.String(), "wrong process event type") &&
				assert.Equal(t, lsExecutable, event.Exe, "wrong executable path") &&
				assert.Equal(t, []string{lsExecutable, "-l"}, event.Cmdline, "wrong command line") &&
				assertNearTimeObject(t, event.ExecTime)
		}, getEventTimeout, lsExecutable, pmodel.Exec)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("exit", func(t *testing.T) {
		sleepExecutable := which(t, "sleep")
		err = test.GetProcessEvent(func() error {
			cmd := exec.Command(sleepExecutable)
			_ = cmd.Run()
			return nil
		}, func(event *pmodel.ProcessEvent) bool {
			return assert.Equal(t, pmodel.Exit.String(), event.EventType.String(), "wrong process event type") &&
				assert.Equal(t, sleepExecutable, event.Exe, "wrong executable path") &&
				assert.Equal(t, []string{sleepExecutable}, event.Cmdline, "wrong command line") &&
				assertNearTimeObject(t, event.ExitTime) &&
				assert.Equal(t, uint32(1), event.ExitCode, "wrong exit code")
		}, getEventTimeout, sleepExecutable, pmodel.Exit)
		if err != nil {
			t.Fatal(err)
		}
	})
}
