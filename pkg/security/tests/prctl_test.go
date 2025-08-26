// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"
	"syscall"
	"testing"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestPrCtl(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_prctl",
			Expression: `prctl.option == PR_SET_NAME`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}

	defer test.Close()

	t.Run("prctl", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			bs, err := syscall.BytePtrFromString("my_thread")
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
			assertTriggeredRule(t, rule, "test_rule_prctl")
		})
	})
}
