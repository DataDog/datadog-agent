// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"

	"github.com/avast/retry-go/v4"
	"github.com/oliveagle/jsonpath"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestVariableAnyField(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{{
		ID: "test_rule_field_variable",
		// TODO(lebauce): should infer event type from variable usage
		Expression: `open.file.path != "" && "%{open.file.path}:foo" == "{{.Root}}/test-open:foo"`,
	}}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	var filename1 string

	test.WaitSignalFromRule(t, func() error {
		filename1, _, err = test.Create("test-open")
		return err
	}, func(_ *model.Event, rule *rules.Rule) {
		assert.Equal(t, "test_rule_field_variable", rule.ID, "wrong rule triggered")
	}, "test_rule_field_variable")
	if err != nil {
		t.Error(err)
	}
	defer os.Remove(filename1)
}

func TestVariablePrivateField(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{{
		ID:         "test_rule_private_variable",
		Expression: `open.file.path == "{{.Root}}/test-private-var"`,
		Actions: []*rules.ActionDefinition{
			{
				Set: &rules.SetDefinition{
					Name:  "public_var",
					Value: true,
				},
			},
			{
				Set: &rules.SetDefinition{
					Name:    "private_var",
					Value:   true,
					Private: true,
				},
			},
		},
	}}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	var filename string

	test.WaitSignalFromRule(t, func() error {
		filename, _, err = test.Create("test-private-var")
		return err
	}, func(_ *model.Event, _ *rules.Rule) {}, "test_rule_private_variable")
	if err != nil {
		t.Error(err)
	}
	defer os.Remove(filename)

	err = retry.Do(func() error {
		msg := test.msgSender.getMsg("test_rule_private_variable")
		if msg == nil {
			return errors.New("message not found")
		}

		jsonPathValidation(test, msg.Data, func(_ *testModule, obj interface{}) {
			if _, err := jsonpath.JsonPathLookup(obj, `$.evt.variables.public_var`); err != nil {
				t.Errorf("public variable should be present in serialized event: %v", err)
			}
			if _, err := jsonpath.JsonPathLookup(obj, `$.evt.variables.private_var`); err == nil {
				t.Errorf("private variable should not be present in serialized event")
			}
		})

		return nil
	})
	if err != nil {
		t.Error(err)
	}
}

// TestVariableInheritanceReparenting verifies that an inherited process-scoped
// variable set on an intermediate process is still visible to its grandchild
// after the intermediate exits and the grandchild is reparented onto a
// subreaper - i.e. that the inherited value is snapshotted onto the grandchild
// before its parent link changes.
func TestVariableInheritanceReparenting(t *testing.T) {
	t.Skip("Need to re-introduce subreaper reparenting")

	SkipIfNotAvailable(t)

	if ebpfLessEnabled {
		t.Skip("subreaper reparenting not supported in ebpfless mode")
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "set_subreaper_var",
			Expression: `open.file.path == "{{.Root}}/subreaper-var-trigger" && process.file.name == "syscall_tester"`,
			Actions: []*rules.ActionDefinition{
				{
					Set: &rules.SetDefinition{
						Scope:     "process",
						Name:      "subreaper_var",
						Value:     "intermediate-value",
						Inherited: true,
					},
				},
			},
		},
		{
			ID: "check_subreaper_var",
			Expression: `open.file.path == "{{.Root}}/subreaper-var-check" ` +
				`&& process.parent.file.name == "syscall_tester" ` +
				`&& ${process.subreaper_var} == "intermediate-value"`,
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

	triggerFile, _, err := test.Path("subreaper-var-trigger")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(triggerFile)

	checkFile, _, err := test.Path("subreaper-var-check")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(checkFile)

	// syscall_tester "subreaper-with-var" forks an intermediate that opens
	// triggerFile (firing set_subreaper_var on the intermediate's scope), then
	// forks a grandchild and exits. After the kernel reparents the grandchild
	// onto the subreaper, the grandchild opens checkFile. The check_subreaper_var
	// rule then asserts that:
	//   - the grandchild's parent is now syscall_tester (the subreaper), and
	//   - ${process.subreaper_var} is still "intermediate-value" - which is
	//     only possible if the value was snapshotted onto the grandchild before
	//     its parent link was redirected away from the (now-gone) intermediate.
	test.WaitSignalFromRule(t, func() error {
		cmd := exec.CommandContext(context.Background(), syscallTester,
			"subreaper-with-var", triggerFile, checkFile)
		return cmd.Run()
	}, func(_ *model.Event, rule *rules.Rule) {
		assertTriggeredRule(t, rule, "check_subreaper_var")
	}, "check_subreaper_var")
}
