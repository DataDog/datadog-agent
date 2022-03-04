// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests
// +build functionaltests

package tests

import (
	"golang.org/x/sys/unix"

	"testing"

	"github.com/stretchr/testify/assert"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestSignalEvent(t *testing.T) {
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

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("test_signal_sigusr1", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(t, syscallTester, "signal", "sigusr")
		}, func(event *sprobe.Event, r *rules.Rule) {
			assert.Equal(t, "signal", event.GetType(), "wrong event type")
			assert.Equal(t, uint32(unix.SIGUSR1), event.Signal.Type, "wrong signal")
			assert.Equal(t, int64(0), event.Signal.Retval, "wrong retval")

			if !validateSignalSchema(t, event) {
				t.Error(event.String())
			}
		})
	})

	t.Run("test_signal_eperm", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(t, syscallTester, "signal", "eperm")
		}, func(event *sprobe.Event, r *rules.Rule) {
			assert.Equal(t, "signal", event.GetType(), "wrong event type")
			assert.Equal(t, uint32(unix.SIGKILL), event.Signal.Type, "wrong signal")
			assert.Equal(t, -int64(unix.EPERM), event.Signal.Retval, "wrong retval")

			if !validateSignalSchema(t, event) {
				t.Error(event.String())
			}
		})
	})
}
