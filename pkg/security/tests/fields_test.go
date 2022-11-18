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

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestFieldsResolver(t *testing.T) {
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_fields_open",
			Expression: `open.file.path == "{{.Root}}/test-fields" && open.flags & O_CREAT != 0`,
		},
		{
			ID:         "test_fields_exec",
			Expression: `exec.file.name == "ls" && exec.argv == "test-fields"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("open", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			_, _, err = test.Create("test-fields")
			return err
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_fields_open")

			event.ResolveFields(false)

			// rely on validateAbnormalPaths
		})
	})

	t.Run("exec", func(t *testing.T) {
		lsExecutable := which(t, "ls")

		test.WaitSignal(t, func() error {
			cmd := exec.Command(lsExecutable, "test-fields")
			_ = cmd.Run()
			return nil
		}, validateExecEvent(t, noWrapperType, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_fields_exec")

			event.ResolveFields(false)

			// rely on validateAbnormalPaths
		}))
	})
}
