// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

package tests

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestSpan(t *testing.T) {
	executable := which(t, "touch")

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_span_rule_open",
			Expression: `open.file.path == "{{.Root}}/test-span"`,
		},
		{
			ID:         "test_span_rule_exec",
			Expression: fmt.Sprintf(`exec.file.path in [ "/usr/bin/touch", "%s" ] && exec.args_flags == "reference"`, executable),
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
				return fmt.Errorf("%s: %w", out, err)
			}

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_span_rule_open")

			test.validateSpanSchema(t, event)

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

		var args []string
		var envs []string
		if kind == dockerWrapperType {
			args = []string{"span-exec", "104", "204", "/usr/bin/touch", "--reference", "/etc/passwd", testFile}
		} else if kind == stdWrapperType {
			args = []string{"span-exec", "104", "204", executable, "--reference", "/etc/passwd", testFile}
		}

		test.WaitSignal(t, func() error {
			cmd := cmdFunc(syscallTester, args, envs)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %w", out, err)
			}

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_span_rule_exec")

			test.validateSpanSchema(t, event)

			assert.Equal(t, uint64(204), event.SpanContext.SpanID)
			assert.Equal(t, uint64(104), event.SpanContext.TraceID)
		})
	})
}
