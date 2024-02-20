// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && functionaltests

// Package tests holds tests related files
package tests

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
)

func TestBasicTest(t *testing.T) {
	ruleDef := &rules.RuleDefinition{
		ID:         "test_basic_rule",
		Expression: `exec.file.name in ["at.exe","schtasks.exe"]`,
	}
	test, err := newTestModule(t, nil, []*rules.RuleDefinition{ruleDef})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("basic process", func(t *testing.T) {
		executable := "schtasks.exe"
		test.WaitSignal(t, func() error {
			cmd := exec.Command(executable)
			return cmd.Run()
		}, test.validateExecEvent(t, noWrapperType, func(event *model.Event, rule *rules.Rule) {
			assertFieldEqualCaseInsensitve(t, event, "exec.file.path", `c:\windows\system32\schtasks.exe`, "wrong exec file path")
			assertFieldIsOneOf(t, event, "process.parent.file.name", []string{"testsuite.exe"}, "wrong process parent file name")
		}))
	})

	test.Run(t, "arg scrubbing", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		inputargs := []string{
			"/create",
			"/u", "execuser",
			"/p", "execpassword",
			"/ru", "runasuser",
			"/rp", "runaspassword",
			"/tn", "test",
			"/tr", "c:\\windows\\system32\\calc.exe",
		}

		test.WaitSignal(t, func() error {
			cmd := cmdFunc("schtasks.exe", inputargs, nil)

			// we will ignore the error.  The username & password arguments are invalid
			_ = cmd.Run()
			return nil
		}, test.validateExecEvent(t, kind, func(event *model.Event, rule *rules.Rule) {
			cmdline, err := event.GetFieldValue("exec.cmdline")
			if err != nil {
				t.Errorf("failed to get exec.cmdline: %v", err)
			}
			// on windows, we don't really get command lines broken into arguments,
			// it's just a whole string.  So things like quotes and escapes are hard.
			// for this test, we have a well known command line, so this will function
			args := strings.Split(cmdline.(string), " ")

			assert.True(t, strings.EqualFold("schtasks.exe", args[0]), "wrong exec.argv0 %s", args[0])

			contains := func(s string) bool {
				for _, arg := range args {
					if s == arg {
						return true
					}
				}
				return false
			}
			if !contains("/u") || !contains("/p") || !contains("/ru") || !contains("/rp") {
				t.Errorf("missing arguments")
			}
			str, err := test.marshalEvent(event)
			if err != nil {
				t.Error(err)
			}
			if !strings.Contains(str, "/p") || !strings.Contains(str, "/rp") {
				t.Error("args not serialized")
			}
			if strings.Contains(str, "execpassword") || strings.Contains(str, "runaspassword") {
				t.Error("args not scrubbed")
			}
		}))
	})
}

func validateProcessContext(tb testing.TB, event *model.Event) {
}

func (tm *testModule) validateExecEvent(tb *testing.T, kind wrapperType, validate func(event *model.Event, rule *rules.Rule)) func(event *model.Event, rule *rules.Rule) {
	return func(event *model.Event, rule *rules.Rule) {
		validate(event, rule)
	}
}

/*
exit event
validating that the cmdline is properly scrubbed (see the linux test)
validating that the process lineage is correct, at least that we have 1 parent
validating the user resolution
*/
