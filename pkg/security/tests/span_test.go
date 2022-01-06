// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests

package tests

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
)

func TestSpan(t *testing.T) {
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_span_rule_open",
			Expression: `open.file.path == "{{.Root}}/test-span"`,
		},
		{
			ID:         "test_span_rule_exec",
			Expression: `exec.file.path == "/usr/bin/touch"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		if _, ok := err.(ErrUnsupportedArch); ok {
			t.Skip(err)
		} else {
			t.Fatal(err)
		}
	}

	test.Run(t, "open", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		testFile, _, err := test.Path("test-span")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		args := []string{"span-open", "104", "204", testFile}
		envs := []string{}

		test.WaitSignal(t, func() error {
			cmd := cmdFunc(syscallTester, args, envs)
			out, err := cmd.CombinedOutput()

			if err != nil {
				//if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %s", out, err)
			}

			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_span_rule_open")

			if !validateSpanSchema(t, event) {
				t.Error(event.String())
			}

			assert.Equal(t, uint64(204), event.SpanContext.SpanID)
			assert.Equal(t, uint64(104), event.SpanContext.TraceID)
		})
	})

	test.Run(t, "exec", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		testFile, _, err := test.Path("test-span-exec")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		args := []string{"span-exec", "104", "204", "/usr/bin/touch", testFile}
		envs := []string{}

		test.WaitSignal(t, func() error {
			cmd := cmdFunc(syscallTester, args, envs)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %s", out, err)
			}

			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_span_rule_exec")

			if !validateSpanSchema(t, event) {
				t.Error(event.String())
			}

			assert.Equal(t, uint64(204), event.SpanContext.SpanID)
			assert.Equal(t, uint64(104), event.SpanContext.TraceID)
		})
	})
}
