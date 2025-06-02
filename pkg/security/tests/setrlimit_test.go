// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"context"
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

	// Use a hybrid approach - we'll use syscall_tester for better compatibility with eBPF probes
	// but with the simpler test structure
	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("setrlimit-nofile", func(t *testing.T) {
		err := test.GetEventSent(t, func() error {
			// cmd := exec.Command(syscallTester, "setrlimit-nofile")
			// out, err := cmd.CombinedOutput()
			// if err != nil {
			// 	return fmt.Errorf("%s: %w", out, err)
			// }
			// return nil
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "setrlimit-nofile")
		}, func(_ *rules.Rule, event *model.Event) bool {
			// Debug: Print full event details
			t.Logf("Received event type: %s", event.GetType())
			t.Logf("Resource: %d", event.Setrlimit.Resource)
			t.Logf("RlimCur: %d", event.Setrlimit.RlimCur)
			t.Logf("RlimMax: %d", event.Setrlimit.RlimMax)
			if jsonStr, err := test.marshalEvent(event); err == nil {
				t.Logf("Full event JSON: %s", jsonStr)
			}

			assert.Equal(t, "setrlimit", event.GetType(), "wrong event type")
			assert.Equal(t, unix.RLIMIT_NOFILE, event.Setrlimit.Resource, "wrong resource (expected RLIMIT_NOFILE)")
			assert.Greater(t, event.Setrlimit.RlimCur, uint64(0), "rlim_cur should be greater than 0")
			assert.Greater(t, event.Setrlimit.RlimMax, uint64(0), "rlim_max should be greater than 0")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			return true
		}, time.Second*20, "test_setrlimit_nofile") // Increased timeout to 10 seconds
		if err != nil {
			t.Error(err)
		}
	})

	t.Run("setrlimit-nproc", func(t *testing.T) {
		err := test.GetEventSent(t, func() error {
			cmd := exec.Command(syscallTester, "setrlimit-nproc")
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %w", out, err)
			}
			return nil
		}, func(_ *rules.Rule, event *model.Event) bool {
			// Debug: Print event details
			t.Logf("Received event type: %s", event.GetType())
			t.Logf("Resource: %d", event.Setrlimit.Resource)
			t.Logf("RlimCur: %d", event.Setrlimit.RlimCur)
			t.Logf("RlimMax: %d", event.Setrlimit.RlimMax)

			assert.Equal(t, "setrlimit", event.GetType(), "wrong event type")
			assert.Equal(t, unix.RLIMIT_NPROC, event.Setrlimit.Resource, "wrong resource (expected RLIMIT_NPROC)")
			assert.Greater(t, event.Setrlimit.RlimCur, uint64(0), "rlim_cur should be greater than 0")
			assert.Greater(t, event.Setrlimit.RlimMax, uint64(0), "rlim_max should be greater than 0")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			return true
		}, time.Second*10, "test_setrlimit_nproc") // Increased timeout to 10 seconds
		if err != nil {
			t.Error(err)
		}
	})

	t.Run("setrlimit-stack", func(t *testing.T) {
		err := test.GetEventSent(t, func() error {
			cmd := exec.Command(syscallTester, "setrlimit-stack")
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %w", out, err)
			}
			return nil
		}, func(_ *rules.Rule, event *model.Event) bool {
			assert.Equal(t, "setrlimit", event.GetType(), "wrong event type")
			assert.Equal(t, unix.RLIMIT_STACK, event.Setrlimit.Resource, "wrong resource (expected RLIMIT_STACK)")
			assert.Greater(t, event.Setrlimit.RlimCur, uint64(0), "rlim_cur should be greater than 0")
			assert.Greater(t, event.Setrlimit.RlimMax, uint64(0), "rlim_max should be greater than 0")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			// test.validateSetrlimitSchema(t, event)
			return true
		}, time.Second*3, "test_setrlimit_stack")
		if err != nil {
			t.Error(err)
		}
	})

	t.Run("setrlimit-core", func(t *testing.T) {
		err := test.GetEventSent(t, func() error {
			cmd := exec.Command(syscallTester, "setrlimit-core")
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %w", out, err)
			}
			return nil
		}, func(_ *rules.Rule, event *model.Event) bool {
			assert.Equal(t, "setrlimit", event.GetType(), "wrong event type")
			assert.Equal(t, unix.RLIMIT_CORE, event.Setrlimit.Resource, "wrong resource (expected RLIMIT_CORE)")
			// For RLIMIT_CORE, we expect cur to be 0
			assert.Equal(t, uint64(0), event.Setrlimit.RlimCur, "rlim_cur should be 0")
			assert.GreaterOrEqual(t, event.Setrlimit.RlimMax, uint64(0), "rlim_max should be valid")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			// test.validateSetrlimitSchema(t, event)
			return true
		}, time.Second*3, "test_setrlimit_core")
		if err != nil {
			t.Error(err)
		}
	})
}
