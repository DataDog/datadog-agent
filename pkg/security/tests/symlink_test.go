// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests
// +build functionaltests

package tests

import (
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestSymlink(t *testing.T) {
	if testEnvironment == DockerEnvironment {
		t.Skip()
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_symlink_1",
			Expression: `open.file.path == "{{.Root}}/test1" && process.file.path == "/usr/bin/python3"`,
		},
		{
			ID:         "test_symlink_2",
			Expression: `open.file.path == "{{.Root}}/test2" && process.file.path in ["/usr/bin/python3"]`,
		},
		{
			ID:         "test_symlink_3",
			Expression: `open.file.path == "{{.Root}}/test3" && process.ancestors.file.path == "/usr/bin/python3"`,
		},
		{
			ID:         "test_symlink_4",
			Expression: `open.file.path == "{{.Root}}/test4" && process.ancestors.file.path in ["/usr/bin/python3"]`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	time.Sleep(2 * time.Second)

	cmdWrapper, err := newDockerCmdWrapper(test.Root(), "python:3.9")
	if err != nil {
		t.Fatal(err)
	}
	defer cmdWrapper.stop()

	cmdWrapper.Run(t, "symlink_1", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		testFile, _, err := test.Path("test1")
		if err != nil {
			t.Fatal(err)
		}

		args := []string{"-c", fmt.Sprintf(`import os; os.open("%s", os.O_CREAT)`, testFile)}
		envs := []string{}

		test.WaitSignal(t, func() error {
			cmd := cmdFunc("/usr/bin/python3", args, envs)
			return cmd.Run()
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_symlink_1")

			assert.Equal(t, "/usr/bin/python3.9", event.ProcessContext.PathnameStr, "wrong symlink")
			assert.Equal(t, testFile, event.Open.File.PathnameStr, "wrong symlink")

			if !validateExecSchema(t, event) {
				t.Error(event.String())
			}
		})
	})

	cmdWrapper.Run(t, "symlink_2", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		testFile, _, err := test.Path("test2")
		if err != nil {
			t.Fatal(err)
		}

		args := []string{"-c", fmt.Sprintf(`import os; os.open("%s", os.O_CREAT)`, testFile)}
		envs := []string{}

		test.WaitSignal(t, func() error {
			cmd := cmdFunc("/usr/bin/python3", args, envs)
			return cmd.Run()
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_symlink_2")

			assert.Equal(t, "/usr/bin/python3.9", event.ProcessContext.PathnameStr, "wrong symlink")
			assert.Equal(t, testFile, event.Open.File.PathnameStr, "wrong symlink")

			if !validateExecSchema(t, event) {
				t.Error(event.String())
			}
		})
	})

	cmdWrapper.Run(t, "symlink_3", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		testFile, _, err := test.Path("test3")
		if err != nil {
			t.Fatal(err)
		}

		args := []string{"-c", fmt.Sprintf(`import os; os.system("touch %s")`, testFile)}
		envs := []string{}

		test.WaitSignal(t, func() error {
			cmd := cmdFunc("/usr/bin/python3", args, envs)
			return cmd.Run()
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_symlink_3")

			assert.Equal(t, "/usr/bin/python3.9", event.ProcessContext.Ancestor.Ancestor.Ancestor.PathnameStr, "wrong symlink")
			assert.Equal(t, testFile, event.Open.File.PathnameStr, "wrong symlink")

			if !validateExecSchema(t, event) {
				t.Error(event.String())
			}
		})
	})

	cmdWrapper.Run(t, "symlink_4", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		testFile, _, err := test.Path("test4")
		if err != nil {
			t.Fatal(err)
		}

		args := []string{"-c", fmt.Sprintf(`import os; os.system("touch %s")`, testFile)}
		envs := []string{}

		test.WaitSignal(t, func() error {
			cmd := cmdFunc("/usr/bin/python3", args, envs)
			return cmd.Run()
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_symlink_4")

			assert.Equal(t, "/usr/bin/python3.9", event.ProcessContext.Ancestor.Ancestor.Ancestor.PathnameStr, "wrong symlink")
			assert.Equal(t, testFile, event.Open.File.PathnameStr, "wrong symlink")

			if !validateExecSchema(t, event) {
				t.Error(event.String())
			}
		})
	})
}
