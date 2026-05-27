// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"encoding/json"
	"fmt"
	"math/big"
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

// splitTraceID parses a decimal 128-bit trace id into (hi, lo) the same way
// secl utils.TraceID stores them.
func splitTraceID(decimalTraceID string) (hi, lo uint64, ok bool) {
	v, parseOK := new(big.Int).SetString(decimalTraceID, 10)
	if !parseOK {
		return 0, 0, false
	}
	mask := new(big.Int).SetUint64(^uint64(0))
	lo = new(big.Int).And(v, mask).Uint64()
	hi = new(big.Int).Rsh(v, 64).Uint64()
	return hi, lo, true
}

// spanContextJSON mirrors the shape of DDContextSerializer in JSON.
type spanContextJSON struct {
	SpanID     string            `json:"span_id"`
	TraceID    string            `json:"trace_id"`
	Attributes map[string]string `json:"attributes"`
}

// spanLocations describes where the per-process span_context is expected to
// surface in the serialized event. The top-level "dd" field is always
// asserted; these flags govern where the per-PCE serializer copy must (or
// must not) appear.
type spanLocations struct {
	// onTopLevelProcess: process.span_context must carry the expected values.
	// Set for non-fork-exec scenarios where fill_span_context captured a
	// span at prepare_binprm and AddExecEntry persisted it on the new PCE.
	onTopLevelProcess bool
	// onAncestor: at least one entry in process.ancestors[].span_context
	// must carry the expected values. Set for fork+exec scenarios where the
	// fork hook captured the parent's span on the child PCE which then
	// became an ancestor of the exec'd image.
	onAncestor bool
}

// assertSerializedSpanContext parses the marshalled event and asserts the
// propagation wiring described by `loc`. Always asserts the top-level "dd"
// field (built by newDDContextSerializer); the rest is gated by loc.
//
// expectedAttrs may be nil; when non-nil, each expected key must be present
// on the asserted span_context.attributes with the expected value (subset
// match — the helper does not assert absence of unexpected keys).
func assertSerializedSpanContext(t *testing.T, jsonStr, expectedSpanID, expectedTraceID string, expectedAttrs map[string]string, loc spanLocations) {
	t.Helper()
	var parsed struct {
		DD struct {
			SpanID  string `json:"span_id"`
			TraceID string `json:"trace_id"`
		} `json:"dd"`
		Process struct {
			SpanContext *spanContextJSON `json:"span_context"`
			Ancestors   []struct {
				SpanContext *spanContextJSON `json:"span_context"`
			} `json:"ancestors"`
		} `json:"process"`
	}
	if !assert.NoError(t, json.Unmarshal([]byte(jsonStr), &parsed), "json.Unmarshal") {
		return
	}

	// (1) Top-level "dd" — always asserted.
	assert.Equal(t, expectedSpanID, parsed.DD.SpanID, "serialized dd.span_id")
	assert.Equal(t, expectedTraceID, parsed.DD.TraceID, "serialized dd.trace_id")

	// (2) Top-level "process.span_context".
	if loc.onTopLevelProcess {
		if assert.NotNil(t, parsed.Process.SpanContext,
			"process.span_context should be populated (AddExecEntry persisted event.SpanContext on the new PCE)") {
			assertSpanFields(t, parsed.Process.SpanContext, expectedSpanID, expectedTraceID, expectedAttrs, "process.span_context")
		}
	} else {
		assert.Nil(t, parsed.Process.SpanContext,
			"process.span_context should be unset (event.SpanContext was zero at exec time; nothing for AddExecEntry to persist)")
	}

	// (3) Ancestor "span_context".
	if loc.onAncestor {
		var ancestorSpan *spanContextJSON
		for i := range parsed.Process.Ancestors {
			if parsed.Process.Ancestors[i].SpanContext != nil {
				ancestorSpan = parsed.Process.Ancestors[i].SpanContext
				break
			}
		}
		if assert.NotNil(t, ancestorSpan,
			"at least one ancestor in process.ancestors[] should carry a serialized span_context") {
			assertSpanFields(t, ancestorSpan, expectedSpanID, expectedTraceID, expectedAttrs, "ancestor.span_context")
		}
	}
}

// assertSpanFields asserts the fields of a serialized span_context match the
// expected span/trace IDs and the expected attribute subset.
func assertSpanFields(t *testing.T, sc *spanContextJSON, expectedSpanID, expectedTraceID string, expectedAttrs map[string]string, prefix string) {
	t.Helper()
	assert.Equal(t, expectedSpanID, sc.SpanID, "%s.span_id", prefix)
	assert.Equal(t, expectedTraceID, sc.TraceID, "%s.trace_id", prefix)
	for k, v := range expectedAttrs {
		assert.Equal(t, v, sc.Attributes[k], "%s.attributes[%q]", prefix, k)
	}
}

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

	// TestSpan exercises the legacy Datadog proprietary TLS path (eRPC
	// RegisterSpanTLSOP populates span_tls; the eBPF probe reads back the
	// trace_id/span_id via that map). That path is gated off by default; opt in
	// here so the runtime constant patches the eBPF program to consult it.
	test, err := newTestModule(t, nil, ruleDefs, withStaticOpts(testOpts{
		legacyAPMCorrelationEnabled: true,
	}))
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

			// In-process exec (pthread + execv preserves the tgid):
			// fill_span_context_legacy reads span_tls[tgid] at prepare_binprm,
			// AddExecEntry persists event.SpanContext onto the new touch PCE,
			// so process.span_context is populated on the top-level process.
			expectedHi, expectedLo, ok := splitTraceID(fakeTraceID128b)
			if !assert.True(t, ok, "splitTraceID") {
				return
			}
			jsonStr, err := test.marshalEvent(event)
			if assert.NoError(t, err, "marshalEvent") {
				assertSerializedSpanContext(t, jsonStr, "204",
					fmt.Sprintf("%x%x", expectedHi, expectedLo), nil,
					spanLocations{onTopLevelProcess: true})
			}
		}, "test_span_rule_exec")
	})

	t.Run("fork_exec_propagates_via_ancestor", func(t *testing.T) {
		// Fork+exec with the legacy proprietary TLS path: the parent
		// registers a span_tls entry (via eRPC), writes span_id/trace_id to
		// its slot, then fork()s. At sched_process_fork the eBPF probe runs
		// in the parent's context, fill_span_context_legacy reads
		// span_tls[parent_tgid] + the parent's TLS slot, and the captured
		// span/trace are saved on the child's ProcessCacheEntry via
		// AddForkEntry → SetSpan. The child execs touch — its own exec
		// event has an empty SpanContext (new tgid, no span_tls entry), but
		// newDDContextSerializer surfaces the parent's span by walking the
		// ancestor chain. This sub-test pins all three points.
		test.RunMultiMode(t, "exec", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
			testFile, _, err := test.Path("test-span-fork-exec")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(testFile)

			var args []string
			if kind == dockerWrapperType {
				args = []string{"span-fork-exec", fakeTraceID128b, "204", "/usr/bin/touch", "--reference", "/etc/passwd", testFile}
			} else {
				args = []string{"span-fork-exec", fakeTraceID128b, "204", executable, "--reference", "/etc/passwd", testFile}
			}

			test.WaitSignalFromRule(t, func() error {
				cmd := cmdFunc(syscallTester, args, []string{})
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}
				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_span_rule_exec")

				// (1) The exec'd program (touch) has no tracer, so the raw
				// exec event SpanContext is empty by design.
				assert.Equal(t, uint64(0), event.SpanContext.SpanID,
					"exec event should not carry a span context: touch has no tracer")
				assert.Equal(t, "0", event.SpanContext.TraceID.String(),
					"exec event should not carry a trace id: touch has no tracer")

				// (2) The fork-parent ancestor should carry the legacy TLS
				// span captured by fill_span_context_legacy at fork time.
				var foundAncestor *model.ProcessCacheEntry
				for pce := event.ProcessContext.Ancestor; pce != nil; pce = pce.Ancestor {
					if pce.SpanContext.SpanID != 0 {
						foundAncestor = pce
						break
					}
				}
				if assert.NotNil(t, foundAncestor,
					"an ancestor should carry the parent's legacy TLS span captured at fork time") {
					assert.Equal(t, uint64(204), foundAncestor.SpanContext.SpanID,
						"fork-parent ancestor SpanID should equal the legacy-TLS span_id")
					assert.Equal(t, fakeTraceID128b, foundAncestor.SpanContext.TraceID.String(),
						"fork-parent ancestor TraceID should equal the legacy-TLS trace_id")
				}

				// (3) Serialized dd field (top-level) and per-process
				// span_context on the ancestor should both carry the
				// propagated legacy-TLS values.
				expectedHi, expectedLo, ok := splitTraceID(fakeTraceID128b)
				if !assert.True(t, ok, "splitTraceID") {
					return
				}
				jsonStr, err := test.marshalEvent(event)
				if assert.NoError(t, err, "marshalEvent") {
					assertSerializedSpanContext(t, jsonStr, "204",
						fmt.Sprintf("%x%x", expectedHi, expectedLo), nil,
						spanLocations{onAncestor: true})
				}
			}, "test_span_rule_exec")
		})
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

	executable := which(t, "touch")

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
		{
			// Shared exec rule for all OTel exec sub-tests. Each sub-test runs
			// sequentially and waits for its own match.
			ID:         "test_otel_span_rule_exec",
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

	// otelExecArgs returns the touch invocation that the exec rule matches.
	// The std and docker modes use a different touch binary path; the rule
	// covers both via `exec.file.path in [ "/usr/bin/touch", "<which>" ]`.
	otelExecArgs := func(kind wrapperType, testFile string) []string {
		touchPath := "/usr/bin/touch"
		if kind == stdWrapperType {
			touchPath = executable
		}
		return []string{touchPath, "--reference", "/etc/passwd", testFile}
	}

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

	t.Run("valid_record_exec", func(t *testing.T) {
		// Exec path: the TLS record is set before execv. fill_exec_context →
		// fill_span_context runs at prepare_binprm, before the new image
		// takes over, so the TLS read still succeeds.
		test.RunMultiMode(t, "exec", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
			testFile, _, err := test.Path("test-otel-span-exec")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(testFile)

			args := append([]string{"otel-span-exec", fakeTraceID128b, "204"}, otelExecArgs(kind, testFile)...)

			test.WaitSignalFromRule(t, func() error {
				cmd := cmdFunc(syscallTester, args, []string{})
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}
				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_otel_span_rule_exec")

				test.validateSpanSchema(t, event)

				assert.Equal(t, "204", strconv.FormatUint(event.SpanContext.SpanID, 10))
				assert.Equal(t, fakeTraceID128b, event.SpanContext.TraceID.String())

				// In-process exec (pthread + execv preserves the tgid): the
				// OTel record is published before execv, fill_span_context_otel
				// reads it at prepare_binprm, AddExecEntry persists
				// event.SpanContext onto the new touch PCE. Top-level
				// process.span_context is populated, including the OTel
				// custom attributes resolved via resolveOTelSpanAttrs.
				expectedHi, expectedLo, ok := splitTraceID(fakeTraceID128b)
				if !assert.True(t, ok, "splitTraceID") {
					return
				}
				jsonStr, err := test.marshalEvent(event)
				if assert.NoError(t, err, "marshalEvent") {
					assertSerializedSpanContext(t, jsonStr, "204",
						fmt.Sprintf("%x%x", expectedHi, expectedLo),
						map[string]string{
							"http.method": "GET",
							"http.target": "/test",
							"http.user":   "will@datadoghq.com",
						},
						spanLocations{onTopLevelProcess: true})
				}
			}, "test_otel_span_rule_exec")
		})
	})

	t.Run("invalid_record_exec", func(t *testing.T) {
		test.RunMultiMode(t, "exec", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
			testFile, _, err := test.Path("test-otel-span-exec-invalid")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(testFile)

			args := append([]string{"otel-span-exec-invalid", fakeTraceID128b, "204"}, otelExecArgs(kind, testFile)...)

			test.WaitSignalFromRule(t, func() error {
				cmd := cmdFunc(syscallTester, args, []string{})
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}
				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_otel_span_rule_exec")

				// valid=0 → no span context.
				assert.Equal(t, uint64(0), event.SpanContext.SpanID)
				assert.Equal(t, "0", event.SpanContext.TraceID.String())
			}, "test_otel_span_rule_exec")
		})
	})

	t.Run("null_pointer_exec", func(t *testing.T) {
		test.RunMultiMode(t, "exec", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
			testFile, _, err := test.Path("test-otel-span-exec-null-ptr")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(testFile)

			args := append([]string{"otel-span-exec-null-ptr"}, otelExecArgs(kind, testFile)...)

			test.WaitSignalFromRule(t, func() error {
				cmd := cmdFunc(syscallTester, args, []string{})
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}
				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_otel_span_rule_exec")

				// NULL TLS pointer → no span context.
				assert.Equal(t, uint64(0), event.SpanContext.SpanID)
				assert.Equal(t, "0", event.SpanContext.TraceID.String())
			}, "test_otel_span_rule_exec")
		})
	})

	t.Run("fork_exec_propagates_via_ancestor", func(t *testing.T) {
		// Fork+exec with the OTel Thread Local Context Record path: the
		// parent's tracer-info memfd triggers ELF dynsym resolution so the
		// agent populates otel_tls[parent_tgid] with the TLS offset, the
		// parent publishes a valid record via the otel_thread_ctx_v1 TLS
		// pointer, then fork()s. At sched_process_fork the eBPF probe runs
		// in the parent's context, fill_span_context_otel reads the
		// parent's thread.fsbase + tls_offset to find the record, and the
		// captured span/trace are saved on the child's ProcessCacheEntry.
		// The child execs touch — its own exec event has an empty
		// SpanContext (new tgid has no otel_tls entry), but
		// newDDContextSerializer surfaces the parent's span by walking the
		// ancestor chain. This sub-test pins all three points.
		test.RunMultiMode(t, "exec", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
			testFile, _, err := test.Path("test-otel-span-fork-exec")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(testFile)

			args := append([]string{"otel-span-fork-exec", fakeTraceID128b, "204"}, otelExecArgs(kind, testFile)...)

			test.WaitSignalFromRule(t, func() error {
				cmd := cmdFunc(syscallTester, args, []string{})
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}
				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_otel_span_rule_exec")

				// (1) The exec'd program (touch) has no tracer, so the raw
				// exec event SpanContext is empty by design.
				assert.Equal(t, uint64(0), event.SpanContext.SpanID,
					"exec event should not carry a span context: touch has no tracer")
				assert.Equal(t, "0", event.SpanContext.TraceID.String(),
					"exec event should not carry a trace id: touch has no tracer")

				// (2) The fork-parent ancestor should carry the OTel TLS
				// span captured by fill_span_context_otel at fork time.
				var foundAncestor *model.ProcessCacheEntry
				for pce := event.ProcessContext.Ancestor; pce != nil; pce = pce.Ancestor {
					if pce.SpanContext.SpanID != 0 {
						foundAncestor = pce
						break
					}
				}
				if assert.NotNil(t, foundAncestor,
					"an ancestor should carry the parent's OTel TLS span captured at fork time") {
					assert.Equal(t, uint64(204), foundAncestor.SpanContext.SpanID,
						"fork-parent ancestor SpanID should equal the OTel record span_id")
					assert.Equal(t, fakeTraceID128b, foundAncestor.SpanContext.TraceID.String(),
						"fork-parent ancestor TraceID should equal the OTel record trace_id")
				}

				// (3) Serialized dd field (top-level) and per-process
				// span_context on the ancestor should both carry the
				// propagated OTel-record values.
				expectedHi, expectedLo, ok := splitTraceID(fakeTraceID128b)
				if !assert.True(t, ok, "splitTraceID") {
					return
				}
				jsonStr, err := test.marshalEvent(event)
				if assert.NoError(t, err, "marshalEvent") {
					assertSerializedSpanContext(t, jsonStr, "204",
						fmt.Sprintf("%x%x", expectedHi, expectedLo), nil,
						spanLocations{onAncestor: true})
				}
			}, "test_otel_span_rule_exec")
		})
	})
}

// TestGoSpan tests Go pprof label-based span context collection.
// dd-trace-go sets goroutine labels "span id" and "local root span id" as decimal strings.
// The eBPF code traverses TLS -> runtime.g -> runtime.m -> curg -> labels to read them.
func TestGoSpan(t *testing.T) {
	SkipIfNotAvailable(t)

	executable := which(t, "touch")

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_go_span_rule_open",
			Expression: `open.file.path == "{{.Root}}/test-go-span"`,
		},
		{
			ID:         "test_go_span_rule_open_no_labels",
			Expression: `open.file.path == "{{.Root}}/test-go-span-no-labels"`,
		},
		{
			ID:         "test_go_span_rule_exec",
			Expression: fmt.Sprintf(`exec.file.path in [ "/usr/bin/touch", "%s" ] && exec.args_flags == "reference"`, executable),
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

	// touchPathFor picks the touch binary path the wrapper-mode expects so the
	// exec rule's `in [ "/usr/bin/touch", "<which>" ]` clause matches.
	touchPathFor := func(kind wrapperType) string {
		if kind == stdWrapperType {
			return executable
		}
		return "/usr/bin/touch"
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

				test.validateSpanSchema(t, event)

				assert.Equal(t, uint64(987654321), event.SpanContext.SpanID,
					"span ID should match the pprof label value")
				assert.Equal(t, uint64(123456789), event.SpanContext.TraceID.Lo,
					"trace ID lo should match the local root span ID label value")
			}, "test_go_span_rule_open")
		})
	})

	t.Run("valid_span_exec", func(t *testing.T) {
		// Set pprof labels then execv touch. fill_span_context_go runs at
		// prepare_binprm — before the image switch — so the goroutine's
		// labels are still readable.
		test.RunMultiMode(t, "exec", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
			testFile, _, err := test.Path("test-go-span-exec")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(testFile)

			args := []string{
				"-go-span-exec-test",
				"-go-span-span-id", "987654321",
				"-go-span-local-root-span-id", "123456789",
				"-go-span-file-path", testFile,
				"-go-span-exec-target", touchPathFor(kind),
			}

			test.WaitSignalFromRule(t, func() error {
				cmd := cmdFunc(goSyscallTester, args, []string{})
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}
				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_go_span_rule_exec")

				test.validateSpanSchema(t, event)

				assert.Equal(t, uint64(987654321), event.SpanContext.SpanID,
					"span ID should match the pprof label value")
				assert.Equal(t, uint64(123456789), event.SpanContext.TraceID.Lo,
					"trace ID lo should match the local root span ID label value")

				// In-process exec via syscall.Exec preserves the tgid:
				// fill_span_context_go reads the goroutine's pprof labels at
				// prepare_binprm, AddExecEntry persists event.SpanContext
				// onto the new touch PCE → process.span_context populated.
				jsonStr, err := test.marshalEvent(event)
				if assert.NoError(t, err, "marshalEvent") {
					assertSerializedSpanContext(t, jsonStr,
						strconv.FormatUint(987654321, 10),
						fmt.Sprintf("%x%x", uint64(0), uint64(123456789)),
						nil,
						spanLocations{onTopLevelProcess: true})
				}
			}, "test_go_span_rule_exec")
		})
	})

	t.Run("no_labels", func(t *testing.T) {
		// Memfd is registered (so the agent resolves Go label offsets) but
		// pprof labels are never set. The eBPF reader should yield an empty
		// span context.
		test.RunMultiMode(t, "open", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
			testFile, _, err := test.Path("test-go-span-no-labels")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(testFile)

			args := []string{
				"-go-span-no-labels-test",
				"-go-span-file-path", testFile,
			}

			test.WaitSignalFromRule(t, func() error {
				cmd := cmdFunc(goSyscallTester, args, []string{})
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}
				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_go_span_rule_open_no_labels")

				assert.Equal(t, uint64(0), event.SpanContext.SpanID)
				assert.Equal(t, "0", event.SpanContext.TraceID.String())
			}, "test_go_span_rule_open_no_labels")
		})
	})

	t.Run("no_labels_exec", func(t *testing.T) {
		test.RunMultiMode(t, "exec", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
			testFile, _, err := test.Path("test-go-span-no-labels-exec")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(testFile)

			args := []string{
				"-go-span-no-labels-exec-test",
				"-go-span-file-path", testFile,
				"-go-span-exec-target", touchPathFor(kind),
			}

			test.WaitSignalFromRule(t, func() error {
				cmd := cmdFunc(goSyscallTester, args, []string{})
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}
				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_go_span_rule_exec")

				assert.Equal(t, uint64(0), event.SpanContext.SpanID)
				assert.Equal(t, "0", event.SpanContext.TraceID.String())
			}, "test_go_span_rule_exec")
		})
	})

	t.Run("fork_exec_propagates_via_ancestor", func(t *testing.T) {
		// Fork+exec is correct-by-design here, not a bug: the exec'd program
		// (touch) has no tracer, so the exec event's own SpanContext must be
		// empty. What carries the parent's span is the fork event:
		// sched_process_fork fires in the PARENT's context, so
		// fill_span_context_go reads the parent's pprof labels and the
		// captured SpanID/TraceID are persisted on the child's
		// ProcessCacheEntry via AddForkEntry → SetSpan.
		//
		// At serialization time, newDDContextSerializer
		// (serializers_linux.go:1457) prefers event.SpanContext, but when
		// that is zero it walks event.ProcessContext.Ancestor and surfaces
		// the first non-zero SpanID/TraceID it finds — which is the fork
		// parent's. So the JSON "dd" field carries the parent's span values
		// even though the raw exec event does not.
		//
		// This sub-test pins all three points of that wiring.
		const parentSpanID uint64 = 987654321
		const parentLocalRootSpanID uint64 = 123456789

		test.RunMultiMode(t, "exec", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
			testFile, _, err := test.Path("test-go-span-fork-exec")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(testFile)

			args := []string{
				"-go-span-fork-exec-test",
				"-go-span-span-id", strconv.FormatUint(parentSpanID, 10),
				"-go-span-local-root-span-id", strconv.FormatUint(parentLocalRootSpanID, 10),
				"-go-span-file-path", testFile,
				"-go-span-exec-target", touchPathFor(kind),
			}

			test.WaitSignalFromRule(t, func() error {
				cmd := cmdFunc(goSyscallTester, args, []string{})
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}
				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_go_span_rule_exec")

				// (1) The exec'd program (touch) has no tracer, so the raw
				// exec event SpanContext is empty by design.
				assert.Equal(t, uint64(0), event.SpanContext.SpanID,
					"exec event should not carry a span context: touch has no tracer")
				assert.Equal(t, "0", event.SpanContext.TraceID.String(),
					"exec event should not carry a trace id: touch has no tracer")

				// (2) The immediate fork-parent in the ancestor lineage
				// should carry the parent's pprof-label span. Walk the
				// ancestor chain like newDDContextSerializer does and
				// confirm we find a PCE with the expected SpanID/TraceID.
				var foundSpan bool
				var ancestorSpanID, ancestorTraceIDLo, ancestorTraceIDHi uint64
				for pce := event.ProcessContext.Ancestor; pce != nil; pce = pce.Ancestor {
					if pce.SpanContext.SpanID != 0 {
						foundSpan = true
						ancestorSpanID = pce.SpanContext.SpanID
						ancestorTraceIDLo = pce.SpanContext.TraceID.Lo
						ancestorTraceIDHi = pce.SpanContext.TraceID.Hi
						break
					}
				}
				assert.True(t, foundSpan,
					"an ancestor should carry the parent's pprof-label span captured at fork time")
				assert.Equal(t, parentSpanID, ancestorSpanID,
					"fork-parent ancestor SpanID should equal the parent's pprof span_id")
				assert.Equal(t, parentLocalRootSpanID, ancestorTraceIDLo,
					"fork-parent ancestor TraceID.Lo should equal the parent's pprof local_root_span_id")
				assert.Equal(t, uint64(0), ancestorTraceIDHi,
					"Go pprof labels only populate the low 64 bits of trace_id")

				// (3) Top-level "dd" field (newDDContextSerializer's ancestor
				// fallback) AND the per-process "span_context" on the
				// ancestor should both carry the parent's pprof-label span.
				jsonStr, err := test.marshalEvent(event)
				if assert.NoError(t, err, "marshalEvent") {
					assertSerializedSpanContext(t, jsonStr,
						strconv.FormatUint(parentSpanID, 10),
						fmt.Sprintf("%x%x", uint64(0), parentLocalRootSpanID),
						nil,
						spanLocations{onAncestor: true})
				}
			}, "test_go_span_rule_exec")
		})
	})
}

// TestDDTraceGoSpan tests the full dd-trace-go integration: dd-trace-go creates
// a real span which internally sets pprof labels ("span id", "local root span id"),
// and the eBPF Go labels reader extracts them from the goroutine's label storage.
func TestDDTraceGoSpan(t *testing.T) {
	SkipIfNotAvailable(t)

	executable := which(t, "touch")

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_ddtrace_span_rule_open",
			Expression: `open.file.path == "{{.Root}}/test-ddtrace-span"`,
		},
		{
			ID:         "test_ddtrace_span_rule_open_no_span",
			Expression: `open.file.path == "{{.Root}}/test-ddtrace-span-no-span"`,
		},
		{
			ID:         "test_ddtrace_span_rule_exec",
			Expression: fmt.Sprintf(`exec.file.path in [ "/usr/bin/touch", "%s" ] && exec.args_flags == "reference"`, executable),
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

	touchPathFor := func(kind wrapperType) string {
		if kind == stdWrapperType {
			return executable
		}
		return "/usr/bin/touch"
	}

	// parseDDTraceIDs scans the tester's stdout for the span/local-root-span
	// IDs that dd-trace-go generated at runtime. Returns (0, 0) when not
	// found (used by the no-span negative path).
	parseDDTraceIDs := func(out []byte) (spanID, lrsID uint64) {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "ddtrace_span_id=") {
				val := strings.TrimPrefix(line, "ddtrace_span_id=")
				spanID, _ = strconv.ParseUint(strings.TrimSpace(val), 10, 64)
			}
			if strings.HasPrefix(line, "ddtrace_local_root_span_id=") {
				val := strings.TrimPrefix(line, "ddtrace_local_root_span_id=")
				lrsID, _ = strconv.ParseUint(strings.TrimSpace(val), 10, 64)
			}
		}
		return spanID, lrsID
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

			var expectedSpanID, expectedLocalRootSpanID uint64

			test.WaitSignalFromRule(t, func() error {
				cmd := cmdFunc(goSyscallTester, args, []string{})
				out, err := cmd.CombinedOutput()
				if err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}
				expectedSpanID, expectedLocalRootSpanID = parseDDTraceIDs(out)
				if expectedSpanID == 0 {
					return fmt.Errorf("failed to parse ddtrace_span_id from output: %s", out)
				}
				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_ddtrace_span_rule_open")

				test.validateSpanSchema(t, event)

				assert.Equal(t, expectedSpanID, event.SpanContext.SpanID,
					"span ID should match the dd-trace-go generated value")
				assert.Equal(t, expectedLocalRootSpanID, event.SpanContext.TraceID.Lo,
					"trace ID lo should match the dd-trace-go local root span ID")
			}, "test_ddtrace_span_rule_open")
		})
	})

	t.Run("ddtrace_span_exec", func(t *testing.T) {
		test.RunMultiMode(t, "exec", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
			testFile, _, err := test.Path("test-ddtrace-span-exec")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(testFile)

			args := []string{
				"-ddtrace-span-exec-test",
				"-ddtrace-span-file-path", testFile,
				"-ddtrace-span-exec-target", touchPathFor(kind),
			}

			var expectedSpanID, expectedLocalRootSpanID uint64

			test.WaitSignalFromRule(t, func() error {
				cmd := cmdFunc(goSyscallTester, args, []string{})
				out, err := cmd.CombinedOutput()
				if err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}
				expectedSpanID, expectedLocalRootSpanID = parseDDTraceIDs(out)
				if expectedSpanID == 0 {
					return fmt.Errorf("failed to parse ddtrace_span_id from output: %s", out)
				}
				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_ddtrace_span_rule_exec")

				test.validateSpanSchema(t, event)

				assert.Equal(t, expectedSpanID, event.SpanContext.SpanID,
					"span ID should match the dd-trace-go generated value")
				assert.Equal(t, expectedLocalRootSpanID, event.SpanContext.TraceID.Lo,
					"trace ID lo should match the dd-trace-go local root span ID")

				// In-process exec via syscall.Exec: dd-trace-go's pprof labels
				// on the locked OS thread are read by fill_span_context_go at
				// prepare_binprm, AddExecEntry persists event.SpanContext
				// onto the new touch PCE → process.span_context populated.
				jsonStr, err := test.marshalEvent(event)
				if assert.NoError(t, err, "marshalEvent") {
					assertSerializedSpanContext(t, jsonStr,
						strconv.FormatUint(expectedSpanID, 10),
						fmt.Sprintf("%x%x", uint64(0), expectedLocalRootSpanID),
						nil,
						spanLocations{onTopLevelProcess: true})
				}
			}, "test_ddtrace_span_rule_exec")
		})
	})

	t.Run("no_span", func(t *testing.T) {
		// dd-trace-go is started but no active span is created. The eBPF
		// reader should yield an empty span context.
		test.RunMultiMode(t, "open", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
			testFile, _, err := test.Path("test-ddtrace-span-no-span")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(testFile)

			args := []string{
				"-ddtrace-no-span-test",
				"-ddtrace-span-file-path", testFile,
			}

			test.WaitSignalFromRule(t, func() error {
				cmd := cmdFunc(goSyscallTester, args, []string{})
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}
				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_ddtrace_span_rule_open_no_span")

				assert.Equal(t, uint64(0), event.SpanContext.SpanID)
				assert.Equal(t, "0", event.SpanContext.TraceID.String())
			}, "test_ddtrace_span_rule_open_no_span")
		})
	})

	t.Run("no_span_exec", func(t *testing.T) {
		test.RunMultiMode(t, "exec", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
			testFile, _, err := test.Path("test-ddtrace-span-no-span-exec")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(testFile)

			args := []string{
				"-ddtrace-no-span-exec-test",
				"-ddtrace-span-file-path", testFile,
				"-ddtrace-span-exec-target", touchPathFor(kind),
			}

			test.WaitSignalFromRule(t, func() error {
				cmd := cmdFunc(goSyscallTester, args, []string{})
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}
				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_ddtrace_span_rule_exec")

				assert.Equal(t, uint64(0), event.SpanContext.SpanID)
				assert.Equal(t, "0", event.SpanContext.TraceID.String())
			}, "test_ddtrace_span_rule_exec")
		})
	})

	t.Run("fork_exec_propagates_via_ancestor", func(t *testing.T) {
		// Fork+exec with a real dd-trace-go span in the parent is
		// correct-by-design, not a bug: the exec'd program (touch) has no
		// tracer, so the exec event's own SpanContext is intentionally
		// empty. The parent's span travels with the fork: sched_process_fork
		// runs in the parent's context, so fill_span_context_go reads the
		// parent's pprof labels and the captured SpanID/TraceID are saved
		// on the child's ProcessCacheEntry via AddForkEntry → SetSpan.
		//
		// newDDContextSerializer (serializers_linux.go:1457) walks
		// event.ProcessContext.Ancestor when event.SpanContext is zero and
		// surfaces the first non-zero SpanID/TraceID it finds — i.e. the
		// fork-parent's. So the serialized "dd" field is populated with the
		// parent's span values.
		//
		// This sub-test pins all three points of that wiring with a real
		// dd-trace-go span.
		test.RunMultiMode(t, "exec", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
			testFile, _, err := test.Path("test-ddtrace-span-fork-exec")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(testFile)

			args := []string{
				"-ddtrace-span-fork-exec-test",
				"-ddtrace-span-file-path", testFile,
				"-ddtrace-span-exec-target", touchPathFor(kind),
			}

			// Read the tester's stdout for the span IDs dd-trace-go
			// generated at runtime; they're the ground truth for what the
			// fork-parent ancestor + serialized dd field should carry.
			var parentSpanID, parentLocalRootSpanID uint64

			test.WaitSignalFromRule(t, func() error {
				cmd := cmdFunc(goSyscallTester, args, []string{})
				out, err := cmd.CombinedOutput()
				if err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}
				parentSpanID, parentLocalRootSpanID = parseDDTraceIDs(out)
				if parentSpanID == 0 {
					return fmt.Errorf("parent dd-trace-go span never produced a non-zero span_id: %s", out)
				}
				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_ddtrace_span_rule_exec")

				// (1) The exec'd program (touch) has no tracer, so the raw
				// exec event SpanContext is empty by design.
				assert.Equal(t, uint64(0), event.SpanContext.SpanID,
					"exec event should not carry a span context: touch has no tracer")
				assert.Equal(t, "0", event.SpanContext.TraceID.String(),
					"exec event should not carry a trace id: touch has no tracer")

				// (2) The immediate fork-parent in the ancestor lineage
				// should carry dd-trace-go's parent span. Walk the chain
				// the same way newDDContextSerializer does.
				var foundSpan bool
				var ancestorSpanID, ancestorTraceIDLo, ancestorTraceIDHi uint64
				for pce := event.ProcessContext.Ancestor; pce != nil; pce = pce.Ancestor {
					if pce.SpanContext.SpanID != 0 {
						foundSpan = true
						ancestorSpanID = pce.SpanContext.SpanID
						ancestorTraceIDLo = pce.SpanContext.TraceID.Lo
						ancestorTraceIDHi = pce.SpanContext.TraceID.Hi
						break
					}
				}
				assert.True(t, foundSpan,
					"an ancestor should carry the dd-trace-go parent span captured at fork time")
				assert.Equal(t, parentSpanID, ancestorSpanID,
					"fork-parent ancestor SpanID should equal dd-trace-go's parent span_id")
				assert.Equal(t, parentLocalRootSpanID, ancestorTraceIDLo,
					"fork-parent ancestor TraceID.Lo should equal dd-trace-go's local_root_span_id")
				assert.Equal(t, uint64(0), ancestorTraceIDHi,
					"dd-trace-go pprof labels only populate the low 64 bits of trace_id")

				// (3) Top-level "dd" field AND the per-process "span_context"
				// on the ancestor should both carry dd-trace-go's parent
				// span values.
				jsonStr, err := test.marshalEvent(event)
				if assert.NoError(t, err, "marshalEvent") {
					assertSerializedSpanContext(t, jsonStr,
						strconv.FormatUint(parentSpanID, 10),
						fmt.Sprintf("%x%x", uint64(0), parentLocalRootSpanID),
						nil,
						spanLocations{onAncestor: true})
				}
			}, "test_ddtrace_span_rule_exec")
		})
	})
}
