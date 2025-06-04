// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"golang.org/x/sys/unix"
)

func TestSetrlimitEvent(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_setrlimit_nofile",
			Expression: `setrlimit.resource == RLIMIT_NOFILE && process.file.name == "syscall_tester"`,
		},
		{
			ID:         "test_setrlimit_nproc",
			Expression: `setrlimit.resource == RLIMIT_NPROC && process.file.name == "syscall_tester"`,
		},
		{
			ID:         "test_setrlimit_stack",
			Expression: `setrlimit.resource == RLIMIT_STACK && process.file.name == "syscall_tester"`,
		},
		{
			ID:         "test_setrlimit_core",
			Expression: `setrlimit.resource == RLIMIT_CORE && process.file.name == "syscall_tester"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("setrlimit-nofile", func(t *testing.T) {
		var expectedPID int
		err := test.GetEventSent(t, func() error {
			cmd := exec.Command(syscallTester, "setrlimit-nofile")
			if err := cmd.Start(); err != nil {
				return fmt.Errorf("failed to start command: %w", err)
			}
			expectedPID = cmd.Process.Pid
			if err := cmd.Wait(); err != nil {
				return fmt.Errorf("command failed: %w", err)
			}
			return nil
		}, func(_ *rules.Rule, event *model.Event) bool {

			assert.Equal(t, "setrlimit", event.GetType(), "wrong event type")
			assert.Equal(t, unix.RLIMIT_NOFILE, event.Setrlimit.Resource, "wrong resource (expected RLIMIT_NOFILE)")
			assert.Equal(t, uint64(1024), event.Setrlimit.RlimCur, "wrong rlim_cur value")
			assert.Equal(t, uint64(2048), event.Setrlimit.RlimMax, "wrong rlim_max value")
			assert.Equal(t, uint32(expectedPID), event.Setrlimit.Target, "wrong target PID")
			assert.Equal(t, int64(0), event.Setrlimit.SyscallEvent.Retval, "retval should be 0 for success")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			return true
		}, time.Second*3, "test_setrlimit_nofile")
		if err != nil {
			t.Error(err)
		}
	})

	t.Run("setrlimit-nproc", func(t *testing.T) {
		var expectedPID int
		err := test.GetEventSent(t, func() error {
			cmd := exec.Command(syscallTester, "setrlimit-nproc")
			if err := cmd.Start(); err != nil {
				return fmt.Errorf("failed to start command: %w", err)
			}
			expectedPID = cmd.Process.Pid
			if err := cmd.Wait(); err != nil {
				return fmt.Errorf("command failed: %w", err)
			}
			return nil
		}, func(_ *rules.Rule, event *model.Event) bool {

			assert.Equal(t, "setrlimit", event.GetType(), "wrong event type")
			assert.Equal(t, unix.RLIMIT_NPROC, event.Setrlimit.Resource, "wrong resource (expected RLIMIT_NPROC)")
			assert.Equal(t, uint64(512), event.Setrlimit.RlimCur, "wrong rlim_cur value")
			assert.Equal(t, uint64(1024), event.Setrlimit.RlimMax, "wrong rlim_max value")
			assert.Equal(t, uint32(expectedPID), event.Setrlimit.Target, "wrong target PID")
			assert.Equal(t, int64(0), event.Setrlimit.SyscallEvent.Retval, "retval should be 0 for success")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			return true
		}, time.Second*3, "test_setrlimit_nproc")
		if err != nil {
			t.Error(err)
		}
	})

	t.Run("setrlimit-stack", func(t *testing.T) {
		var parentPID int
		err := test.GetEventSent(t, func() error {
			cmd := exec.Command(syscallTester, "setrlimit-stack")
			if err := cmd.Start(); err != nil {
				return fmt.Errorf("failed to start command: %w", err)
			}
			parentPID = cmd.Process.Pid
			t.Logf("Parent process PID: %d", parentPID)
			if err := cmd.Wait(); err != nil {
				return fmt.Errorf("command failed: %w", err)
			}
			return nil
		}, func(_ *rules.Rule, event *model.Event) bool {

			assert.Equal(t, "setrlimit", event.GetType(), "wrong event type")
			assert.Equal(t, unix.RLIMIT_STACK, event.Setrlimit.Resource, "wrong resource (expected RLIMIT_STACK)")
			assert.Equal(t, uint64(8192*1024), event.Setrlimit.RlimCur, "wrong rlim_cur value")
			assert.Equal(t, uint64(16384*1024), event.Setrlimit.RlimMax, "wrong rlim_max value")
			assert.Equal(t, int64(0), event.Setrlimit.SyscallEvent.Retval, "retval should be 0 for success")
			assert.Equal(t, event.Setrlimit.Target, uint32(parentPID)+1, "target PID should be different from parent PID")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			return true
		}, time.Second*3, "test_setrlimit_stack")
		if err != nil {
			t.Error(err)
		}
	})

	t.Run("setrlimit-core", func(t *testing.T) {
		var expectedPID int
		err := test.GetEventSent(t, func() error {
			cmd := exec.Command(syscallTester, "setrlimit-core")
			if err := cmd.Start(); err != nil {
				return fmt.Errorf("failed to start command: %w", err)
			}
			expectedPID = cmd.Process.Pid
			if err := cmd.Wait(); err != nil {
				return fmt.Errorf("command failed: %w", err)
			}
			return nil
		}, func(_ *rules.Rule, event *model.Event) bool {

			assert.Equal(t, "setrlimit", event.GetType(), "wrong event type")
			assert.Equal(t, unix.RLIMIT_CORE, event.Setrlimit.Resource, "wrong resource (expected RLIMIT_CORE)")
			assert.Equal(t, uint64(0), event.Setrlimit.RlimCur, "wrong rlim_cur value")
			assert.Equal(t, uint64(0), event.Setrlimit.RlimMax, "wrong rlim_max value")
			assert.Equal(t, uint32(expectedPID), event.Setrlimit.Target, "wrong target PID")
			assert.Equal(t, int64(0), event.Setrlimit.SyscallEvent.Retval, "retval should be 0 for success")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			return true
		}, time.Second*3, "test_setrlimit_core")
		if err != nil {
			t.Error(err)
		}
	})
}
