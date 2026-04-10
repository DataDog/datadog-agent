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
	"strings"
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

// TestGoSpan tests Go pprof label-based span context collection.
// dd-trace-go sets goroutine labels "span id" and "local root span id" as decimal strings.
// The eBPF code traverses TLS -> runtime.g -> runtime.m -> curg -> labels to read them.
func TestGoSpan(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_go_span_rule_open",
			Expression: `open.file.path == "{{.Root}}/test-go-span"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	goSyscallTester, err := loadSyscallTester(t, test, "syscall_go_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("valid_span", func(t *testing.T) {
		test.RunMultiMode(t, "open", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
			testFile, _, err := test.Path("test-go-span")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(testFile)

			args := []string{
				"-go-span-test",
				"-go-span-span-id", "987654321",
				"-go-span-local-root-span-id", "123456789",
				"-go-span-file-path", testFile,
			}
			envs := []string{}

			test.WaitSignalFromRule(t, func() error {
				cmd := cmdFunc(goSyscallTester, args, envs)
				out, err := cmd.CombinedOutput()

				if err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}

				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_go_span_rule_open")

				assert.Equal(t, uint64(987654321), event.SpanContext.SpanID,
					"span ID should match the pprof label value")
				assert.Equal(t, uint64(123456789), event.SpanContext.TraceID.Lo,
					"trace ID lo should match the local root span ID label value")
			}, "test_go_span_rule_open")
		})
	})
}

// TestDDTraceGoSpan tests the full dd-trace-go integration: dd-trace-go creates
// a real span which internally sets pprof labels ("span id", "local root span id"),
// and the eBPF Go labels reader extracts them from the goroutine's label storage.
func TestDDTraceGoSpan(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_ddtrace_span_rule_open",
			Expression: `open.file.path == "{{.Root}}/test-ddtrace-span"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	goSyscallTester, err := loadSyscallTester(t, test, "syscall_go_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("ddtrace_span", func(t *testing.T) {
		test.RunMultiMode(t, "open", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
			testFile, _, err := test.Path("test-ddtrace-span")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(testFile)

			args := []string{
				"-ddtrace-span-test",
				"-ddtrace-span-file-path", testFile,
			}
			envs := []string{}

			// Capture the tester's stdout to extract the span IDs
			// that dd-trace-go generated at runtime.
			var expectedSpanID, expectedLocalRootSpanID uint64

			test.WaitSignalFromRule(t, func() error {
				cmd := cmdFunc(goSyscallTester, args, envs)
				out, err := cmd.CombinedOutput()

				if err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}

				// Parse the span IDs from the tester's output.
				for _, line := range strings.Split(string(out), "\n") {
					if strings.HasPrefix(line, "ddtrace_span_id=") {
						val := strings.TrimPrefix(line, "ddtrace_span_id=")
						expectedSpanID, _ = strconv.ParseUint(strings.TrimSpace(val), 10, 64)
					}
					if strings.HasPrefix(line, "ddtrace_local_root_span_id=") {
						val := strings.TrimPrefix(line, "ddtrace_local_root_span_id=")
						expectedLocalRootSpanID, _ = strconv.ParseUint(strings.TrimSpace(val), 10, 64)
					}
				}

				if expectedSpanID == 0 {
					return fmt.Errorf("failed to parse ddtrace_span_id from output: %s", out)
				}

				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_ddtrace_span_rule_open")

				assert.Equal(t, expectedSpanID, event.SpanContext.SpanID,
					"span ID should match the dd-trace-go generated value")
				assert.Equal(t, expectedLocalRootSpanID, event.SpanContext.TraceID.Lo,
					"trace ID lo should match the dd-trace-go local root span ID")
			}, "test_ddtrace_span_rule_open")
		})
	})
}
