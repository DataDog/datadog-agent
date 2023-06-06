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

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestArithmeticOperation(t *testing.T) {

	// Need to add additional conditions so that the event type can be inferred
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_simple_addition",
			Expression: `1 + 2 == 5 - 2 && exec.comm in ["ls"]`,
		},
		{
			ID:         "test_simple_addition_false",
			Expression: `1 + 2 != 3 && exec.comm in ["ls"]`,
		},
		{
			ID:         "test_more_complex",
			Expression: `1 + 2 - 3 + 4  == 4 && exec.comm in ["cp"]`,
		},
		{
			ID:         "test_with_parentheses",
			Expression: `1 - 2 + 3 - (1 - 4) - (1 - 5) == 9 && exec.comm in ["pwd"]`,
		},
		{
			ID:         "test_with_time",
			Expression: `10s + 40s == 50s && exec.comm in ["cat"]`,
		},
		{
			ID:         "test_with_time_2",
			Expression: `process.created_at < 5s && exec.comm in ["grep"]`,
		},
		{
			ID:         "test_with_time_3",
			Expression: `event.timestamp - process.created_at + 3s <= 5s && exec.comm in ["echo"]`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("test_simple_addition", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			cmd := exec.Command("ls")
			cmd.Run()
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_simple_addition")
		})
	})
	t.Run("test_simple_addition_false", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			cmd := exec.Command("ls")
			cmd.Run()
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertNotTriggeredRule(t, rule, "test_simple_addition_false")
		})
	})

	t.Run("test_more_complex", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			cmd := exec.Command("cp")
			cmd.Run()
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_more_complex")
		})
	})

	t.Run("test_with_parentheses", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			cmd := exec.Command("pwd")
			cmd.Run()
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_with_parentheses")
		})
	})

	t.Run("test_with_time", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			cmd := exec.Command("cat")
			cmd.Run()
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_with_time")
		})
	})

	t.Run("test_with_time_2", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			cmd := exec.Command("grep")
			cmd.Run()
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_with_time_2")
		})
	})

	t.Run("test_with_time_3", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			cmd := exec.Command("echo")
			cmd.Run()
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_with_time_3")
		})
	})
}
