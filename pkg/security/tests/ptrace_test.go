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
)

func TestPTraceEvent(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_ptrace_cont",
			Expression: `ptrace.request == PTRACE_CONT && ptrace.tracee.file.name == "syscall_tester"`,
		},
		{
			ID:         "test_ptrace_me",
			Expression: `ptrace.request == PTRACE_TRACEME && process.file.name == "syscall_tester"`,
		},
		{
			ID:         "test_ptrace_attach",
			Expression: `ptrace.request == PTRACE_ATTACH && ptrace.tracee.file.name == "syscall_tester"`,
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

	test.RunMultiMode(t, "ptrace-cont", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		args := []string{"ptrace-traceme"}
		envs := []string{}

		err := test.GetEventSent(t, func() error {
			cmd := cmdFunc(syscallTester, args, envs)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %w", out, err)
			}

			return nil
		}, func(_ *rules.Rule, event *model.Event) bool {
			assert.Equal(t, "ptrace", event.GetType(), "wrong event type")
			assert.Equal(t, uint64(42), event.PTrace.Address, "wrong address")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validatePTraceSchema(t, event)
			return true
		}, time.Second*3, "test_ptrace_cont")
		if err != nil {
			t.Error(err)
		}
	})

	test.RunMultiMode(t, "ptrace-me", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		args := []string{"ptrace-traceme"}
		envs := []string{}

		err := test.GetEventSent(t, func() error {
			cmd := cmdFunc(syscallTester, args, envs)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %w", out, err)
			}

			return nil
		}, func(_ *rules.Rule, event *model.Event) bool {
			assert.Equal(t, "ptrace", event.GetType(), "wrong event type")
			assert.Equal(t, uint64(0), event.PTrace.Address, "wrong address")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validatePTraceSchema(t, event)
			return true
		}, time.Second*3, "test_ptrace_me")
		if err != nil {
			t.Error(err)
		}
	})

	test.RunMultiMode(t, "ptrace-attach", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		args := []string{"sleep", "2", ";", "ptrace-attach"}
		envs := []string{}

		err := test.GetEventSent(t, func() error {
			cmd := cmdFunc(syscallTester, args, envs)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %w", out, err)
			}

			return nil
		}, func(_ *rules.Rule, event *model.Event) bool {
			assert.Equal(t, "ptrace", event.GetType(), "wrong event type")
			assert.Equal(t, uint64(0), event.PTrace.Address, "wrong address")
			assert.Equal(t, event.PTrace.Tracee.PPid, event.PTrace.Tracee.Parent.Pid, "tracee wrong ppid / parent pid")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validatePTraceSchema(t, event)
			return true
		}, time.Second*6, "test_ptrace_attach")
		if err != nil {
			t.Error(err)
		}
	})
}
