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
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
	"syscall"
	"testing"
	"time"
)

func TestPrCtl(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_prctl",
			Expression: `prctl.option == PR_SET_NAME && prctl.new_name == "my_thread"`,
		},
		{
			ID:         "test_rule_prctl_get_dumpable",
			Expression: `prctl.option == PR_GET_DUMPABLE`,
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

	t.Run("prctl-set-name", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "prctl-setname", "my_thread")
		}, func(_ *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_prctl")
		}, "test_rule_prctl")
	})
	t.Run("prctl-get-dumpable", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			_, _, errno := syscall.Syscall6(
				syscall.SYS_PRCTL,
				uintptr(syscall.PR_GET_DUMPABLE),
				0, 0, 0, 0, 0,
			)
			if errno != 0 {
				return fmt.Errorf("prctl(PR_GET_DUMPABLE) failed: %v", errno)
			}
			return nil
		}, func(_ *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_prctl_get_dumpable")
		}, "test_rule_prctl_get_dumpable")
	})
}

func TestPrCtlTruncated(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_prctl_truncated",
			Expression: `prctl.option == PR_SET_NAME && prctl.is_name_truncated == true`,
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

	t.Run("prctl-set-name-truncated", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "prctl-setname", "my_thread_is_waaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaay_too_long")
		}, func(_ *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_prctl_truncated")
		}, "test_rule_prctl_truncated")
		test.eventMonitor.SendStats()
		key := metrics.MetricNameTruncated
		assert.NotEmpty(t, test.statsdClient.Get(key))
		assert.NotZero(t, test.statsdClient.Get(key))
	})
}

func TestPrCtlDiscarder(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_prctl",
			Expression: `prctl.option == PR_SET_NAME && prctl.new_name == "my_thread"`,
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

	truncate := func(s string, max int) string {
		if len(s) <= max {
			return s
		}
		return s[:max]
	}

	testPrName := func(t *testing.T, prName string) {
		t.Helper()
		// 1. send first event not matching the rule to create a discarder.
		// it's expected that we receive the event
		err = test.GetProbeEvent(func() error {
			err = runSyscallTesterFunc(context.Background(), t, syscallTester, "prctl-setname", prName)
			if err != nil {
				t.Fatal("error calling prctl: %w", err)
			}
			return nil
		}, func(event *model.Event) bool {
			if event.GetType() != "prctl" || event.PrCtl.NewName != truncate(prName, 15) {
				return false
			}

			return true
		}, 3*time.Second, model.PrCtlEventType)

		if err != nil {
			t.Fatal("Failed to get the first prctl event")
		}

		// 2. make sure that we're still receiving non-discarded messages
		test.WaitSignalFromRule(t, func() error {
			err = runSyscallTesterFunc(context.Background(), t, syscallTester, "prctl-setname", "my_thread")
			if err != nil {
				t.Fatal("error calling prctl: %w", err)
			}
			return nil
		}, func(_ *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_prctl")
		}, "test_rule_prctl")

		// 3. trigger prctl again with the event that shall be discarded and check that we don't receive anything
		err = test.GetProbeEvent(func() error {
			err = runSyscallTesterFunc(context.Background(), t, syscallTester, "prctl-setname", prName)
			if err != nil {
				t.Fatal("error calling prctl: %w", err)
			}
			return nil
		}, func(event *model.Event) bool {
			if event.GetType() != "prctl" || event.PrCtl.NewName != truncate(prName, 15) {
				return false
			}
			return true
		}, 3*time.Second, model.PrCtlEventType)

		assert.NotEqual(t, err, nil, "Event wasn't discarded")
	}

	t.Run("prctl-set-name-discarder-empty", func(t *testing.T) {
		testPrName(t, "")
	})

	t.Run("prctl-set-name-discarder-small", func(t *testing.T) {
		testPrName(t, "bidule")
	})

	t.Run("prctl-set-name-discarder-limit", func(t *testing.T) {
		testPrName(t, "0123456789ABCDE")
	})

	t.Run("prctl-set-name-discarder-above-limit", func(t *testing.T) {
		testPrName(t, "looooooooooooooooooooooong")
	})
}

func TestPrCtlDiscarderReload(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_prctl",
			Expression: `prctl.option == PR_SET_NAME && prctl.new_name == "my_thread"`,
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

	truncate := func(s string, max int) string {
		if len(s) <= max {
			return s
		}
		return s[:max]
	}

	p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		t.Skip("not supported")
	}

	t.Run("prctl-discarders-invalidated-when-ruleset-is-reloaded", func(t *testing.T) {
		prName := "cenas"
		t.Helper()
		// 1. send first event not matching the rule to create a discarder.
		// it's expected that we receive the event
		err = test.GetProbeEvent(func() error {
			err = runSyscallTesterFunc(context.Background(), t, syscallTester, "prctl-setname", prName)
			if err != nil {
				t.Fatal("error calling prctl: %w", err)
			}
			return nil
		}, func(event *model.Event) bool {
			if event.GetType() != "prctl" || event.PrCtl.NewName != truncate(prName, 15) {
				return false
			}

			return true
		}, 3*time.Second, model.PrCtlEventType)

		if err != nil {
			t.Fatal("Failed to get the first prctl event")
		}

		// 2. make sure that we're still receiving non-discarded messages
		test.WaitSignalFromRule(t, func() error {
			err = runSyscallTesterFunc(context.Background(), t, syscallTester, "prctl-setname", "my_thread")
			if err != nil {
				t.Fatal("error calling prctl: %w", err)
			}
			return nil
		}, func(_ *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_prctl")
		}, "test_rule_prctl")

		// 3. flush the discarders
		assert.NoError(t, p.FlushDiscarders())

		// 4. trigger prctl again with the name that had previously been added to the discarders. we should receive it now
		err = test.GetProbeEvent(func() error {
			err = runSyscallTesterFunc(context.Background(), t, syscallTester, "prctl-setname", prName)
			if err != nil {
				t.Fatal("error calling prctl: %w", err)
			}
			return nil
		}, func(event *model.Event) bool {
			if event.GetType() != "prctl" || event.PrCtl.NewName != truncate(prName, 15) {
				return false
			}
			return true
		}, 3*time.Second, model.PrCtlEventType)

		assert.Equal(t, err, nil, "Should have received the event")
	})
}
