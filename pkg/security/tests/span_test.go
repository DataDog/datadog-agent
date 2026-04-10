// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestSpan(t *testing.T) {
	SkipIfNotAvailable(t)

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

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	fakeTraceID128b := "136272290892501783905308705057321818530"

	test.RunMultiMode(t, "open", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		testFile, _, err := test.Path("test-span")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		args := []string{"span-open", fakeTraceID128b, "204", testFile}
		envs := []string{}

		test.WaitSignalFromRule(t, func() error {
			cmd := cmdFunc(syscallTester, args, envs)
			out, err := cmd.CombinedOutput()

			if err != nil {
				return fmt.Errorf("%s: %w", out, err)
			}

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_span_rule_open")

			test.validateSpanSchema(t, event)

			assert.Equal(t, "204", strconv.FormatUint(event.SpanContext.SpanID, 10))
			assert.Equal(t, fakeTraceID128b, event.SpanContext.TraceID.String())
		}, "test_span_rule_open")
	})

	test.RunMultiMode(t, "exec", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		testFile, _, err := test.Path("test-span-exec")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		var args []string
		var envs []string
		if kind == dockerWrapperType {
			args = []string{"span-exec", fakeTraceID128b, "204", "/usr/bin/touch", "--reference", "/etc/passwd", testFile}
		} else if kind == stdWrapperType {
			args = []string{"span-exec", fakeTraceID128b, "204", executable, "--reference", "/etc/passwd", testFile}
		}

		test.WaitSignalFromRule(t, func() error {
			cmd := cmdFunc(syscallTester, args, envs)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %w", out, err)
			}

			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_span_rule_exec")

			test.validateSpanSchema(t, event)

			assert.Equal(t, "204", strconv.FormatUint(event.SpanContext.SpanID, 10))
			assert.Equal(t, fakeTraceID128b, event.SpanContext.TraceID.String())
		}, "test_span_rule_exec")
	})
}

// TestOTelSpan tests OTel Thread Local Context Record based span context collection.
// This tests the native application TLSDESC path (per OTel spec PR #4947).
// Only supported on x86_64 (reads fsbase from task_struct->thread.fsbase).
func TestOTelSpan(t *testing.T) {
	SkipIfNotAvailable(t)

	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("OTel TLSDESC span test only supported on amd64 and arm64")
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_otel_span_rule_open",
			Expression: `open.file.path == "{{.Root}}/test-otel-span"`,
		},
		{
			ID:         "test_otel_span_rule_open_invalid",
			Expression: `open.file.path == "{{.Root}}/test-otel-span-invalid"`,
		},
		{
			ID:         "test_otel_span_rule_open_null_ptr",
			Expression: `open.file.path == "{{.Root}}/test-otel-span-null-ptr"`,
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

	fakeTraceID128b := "136272290892501783905308705057321818530"

	t.Run("valid_record", func(t *testing.T) {
		test.RunMultiMode(t, "open", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
			testFile, _, err := test.Path("test-otel-span")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(testFile)

			args := []string{"otel-span-open", fakeTraceID128b, "204", testFile}
			envs := []string{}

			test.WaitSignalFromRule(t, func() error {
				cmd := cmdFunc(syscallTester, args, envs)
				out, err := cmd.CombinedOutput()

				if err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}

				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_otel_span_rule_open")

				test.validateSpanSchema(t, event)

				assert.Equal(t, "204", strconv.FormatUint(event.SpanContext.SpanID, 10))
				assert.Equal(t, fakeTraceID128b, event.SpanContext.TraceID.String())

				// Verify custom OTel attributes were parsed from attrs_data.
				assert.NotNil(t, event.SpanContext.Attributes, "attributes should be non-nil")
				assert.Equal(t, "GET", event.SpanContext.Attributes["http.method"],
					"http.method attribute should be GET")
				assert.Equal(t, "/test", event.SpanContext.Attributes["http.target"],
					"http.target attribute should be /test")
				assert.Equal(t, "will@datadoghq.com", event.SpanContext.Attributes["http.user"],
					"http.user attribute should be will@datadoghq.com")
			}, "test_otel_span_rule_open")
		})
	})

	t.Run("invalid_record", func(t *testing.T) {
		// Tests that the eBPF reader rejects a record with valid=0 and returns zero span context.
		test.RunMultiMode(t, "open", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
			testFile, _, err := test.Path("test-otel-span-invalid")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(testFile)

			args := []string{"otel-span-open-invalid", fakeTraceID128b, "204", testFile}
			envs := []string{}

			test.WaitSignalFromRule(t, func() error {
				cmd := cmdFunc(syscallTester, args, envs)
				out, err := cmd.CombinedOutput()

				if err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}

				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_otel_span_rule_open_invalid")

				// The record has valid=0, so span context must be zero.
				assert.Equal(t, uint64(0), event.SpanContext.SpanID)
				assert.Equal(t, "0", event.SpanContext.TraceID.String())
			}, "test_otel_span_rule_open_invalid")
		})
	})

	t.Run("null_pointer", func(t *testing.T) {
		// Tests that the eBPF reader handles a NULL TLS pointer gracefully (zero span context).
		test.RunMultiMode(t, "open", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
			testFile, _, err := test.Path("test-otel-span-null-ptr")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(testFile)

			args := []string{"otel-span-open-null-ptr", testFile}
			envs := []string{}

			test.WaitSignalFromRule(t, func() error {
				cmd := cmdFunc(syscallTester, args, envs)
				out, err := cmd.CombinedOutput()

				if err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}

				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_otel_span_rule_open_null_ptr")

				// The TLS pointer is NULL, so span context must be zero.
				assert.Equal(t, uint64(0), event.SpanContext.SpanID)
				assert.Equal(t, "0", event.SpanContext.TraceID.String())
			}, "test_otel_span_rule_open_null_ptr")
		})
	})
}
