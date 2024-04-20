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

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestSignalEvent(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_signal_sigusr1",
			Expression: `signal.type == SIGUSR1 && signal.target.file.name == "syscall_tester"`,
		},
		{
			ID:         "test_signal_eperm",
			Expression: `signal.type == SIGKILL && signal.target.file.name == "syscall_tester" && signal.retval == EPERM`,
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

	test.Run(t, "signal-sigusr1", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		args := []string{"signal", "sigusr1"}
		envs := []string{}

		test.WaitSignal(t, func() error {
			cmd := cmdFunc(syscallTester, args, envs)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %w", out, err)
			}

			return nil
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "signal", event.GetType(), "wrong event type")
			assert.Equal(t, uint32(unix.SIGUSR1), event.Signal.Type, "wrong signal")
			assert.Equal(t, int64(0), event.Signal.Retval, "wrong retval")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validateSignalSchema(t, event)
		})
	})

	test.Run(t, "signal-eperm", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		args := []string{"signal", "eperm"}
		envs := []string{}

		test.WaitSignal(t, func() error {
			cmd := cmdFunc(syscallTester, args, envs)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %w", out, err)
			}

			return nil
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "signal", event.GetType(), "wrong event type")
			assert.Equal(t, uint32(unix.SIGKILL), event.Signal.Type, "wrong signal")
			assert.Equal(t, -int64(unix.EPERM), event.Signal.Retval, "wrong retval")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validateSignalSchema(t, event)
		})
	})
}
