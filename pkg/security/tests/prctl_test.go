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
	"syscall"
	"testing"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
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
		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "prctl-setname", "my_thread")
		}, func(_ *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_prctl")
		})
	})
	t.Run("prctl-get-dumpable", func(t *testing.T) {
		test.WaitSignal(t, func() error {
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
		})
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
	t.Run("prctl-set-name-truncated", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			bs, err := syscall.BytePtrFromString("my_thread_is_waaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaay_too_long")
			if err != nil {
				return fmt.Errorf("failed to convert string: %v", err)
			}

			_, _, errno := syscall.Syscall6(
				syscall.SYS_PRCTL,
				uintptr(syscall.PR_SET_NAME),
				uintptr(unsafe.Pointer(bs)), 0, 0, 0, 0,
			)
			if errno != 0 {
				return fmt.Errorf("prctl failed: %v", errno)
			}
			return nil
		}, func(_ *model.Event, rule *rules.Rule) {

			assertTriggeredRule(t, rule, "test_rule_prctl_truncated")
		})
		test.eventMonitor.SendStats()
		key := metrics.MetricNameTruncated
		assert.NotEmpty(t, test.statsdClient.Get(key))
		assert.NotZero(t, test.statsdClient.Get(key))

	})
}
