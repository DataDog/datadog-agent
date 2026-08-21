// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/utils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// skipIfNoThreadPointer skips tests that need the TLS thread pointer read out of
// task_struct->thread which requires offsets that only exists from 4.7 up.
func skipIfNoThreadPointer(t *testing.T) {
	t.Helper()

	checkKernelCompatibility(t, "thread pointer offsets require kernel 4.7+", func(kv *kernel.Version) bool {
		return kv.Code < kernel.Kernel4_7
	})
}

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

// traceJSON mirrors the shape of SpanContextSerializer in JSON.
type traceJSON struct {
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

// processTracerJSON mirrors the serialized "tracer" wrapper on a process node:
// a "trace" span context plus optional tracer metadata.
type processTracerJSON struct {
	Trace *traceJSON `json:"trace"`
}

// spanJSON mirrors the span-carrying parts of a serialized event.
type spanJSON struct {
	DD      *traceJSON `json:"dd"`
	Trace   *traceJSON `json:"trace"`
	Process struct {
		Tracer    *processTracerJSON `json:"tracer"`
		Ancestors []struct {
			Tracer *processTracerJSON `json:"tracer"`
		} `json:"ancestors"`
	} `json:"process"`
}

// parseSpanJSON unmarshals the span-carrying parts of a marshalled event.
func parseSpanJSON(t *testing.T, jsonStr string) (spanJSON, bool) {
	t.Helper()
	var parsed spanJSON
	if !assert.NoError(t, json.Unmarshal([]byte(jsonStr), &parsed), "json.Unmarshal") {
		return parsed, false
	}
	return parsed, true
}

// assertSerializedTrace asserts the two top-level span keys of a marshalled
// event: "dd" (intake-consumed) and "trace" (user-facing). Both are populated
// from the same serializer instance in EventSerializer, so any divergence
// between them would indicate a serialization bug.
//
// expectedAttrs may be nil; when non-nil, each expected key must be present
// with the expected value (subset match — absence of unexpected keys is not
// asserted).
func assertSerializedTrace(t *testing.T, jsonStr, expectedSpanID, expectedTraceID string, expectedAttrs map[string]string) {
	t.Helper()
	parsed, ok := parseSpanJSON(t, jsonStr)
	if !ok {
		return
	}

	if assert.NotNil(t, parsed.DD, "serialized dd field should be populated") {
		assertSpanFields(t, parsed.DD, expectedSpanID, expectedTraceID, expectedAttrs, "dd")
	}
	if assert.NotNil(t, parsed.Trace, "serialized trace field should be populated") {
		assertSpanFields(t, parsed.Trace, expectedSpanID, expectedTraceID, expectedAttrs, "trace")
	}
}

// assertNoSerializedTrace asserts that no span context surfaced in the
// marshalled event. newTraceSerializer returns nil when neither the event nor
// any ancestor carries one, which leaves both omitempty keys out of the JSON
// entirely.
func assertNoSerializedTrace(t *testing.T, jsonStr string) {
	t.Helper()
	parsed, ok := parseSpanJSON(t, jsonStr)
	if !ok {
		return
	}

	assert.Nil(t, parsed.DD, "serialized dd field should be omitted: nothing in the lineage carries a span")
	assert.Nil(t, parsed.Trace, "serialized trace field should be omitted: nothing in the lineage carries a span")
}

// assertSerializedSpanContext parses the marshalled event and asserts the
// propagation wiring described by `loc`. Always asserts the top-level "dd"
// and "trace" fields; the per-process copies are gated by loc.
//
// expectedAttrs is matched as described on assertSerializedTrace.
func assertSerializedSpanContext(t *testing.T, jsonStr, expectedSpanID, expectedTraceID string, expectedAttrs map[string]string, loc spanLocations) {
	t.Helper()
	parsed, ok := parseSpanJSON(t, jsonStr)
	if !ok {
		return
	}

	// (1) Top-level "dd" and "trace" — always asserted.
	assertSerializedTrace(t, jsonStr, expectedSpanID, expectedTraceID, expectedAttrs)

	// (2) "process.tracer.trace" — populated when AddExecEntry persisted
	// event.SpanContext onto the new PCE (in-process exec scenarios).
	if loc.onTopLevelProcess {
		if assert.NotNil(t, parsed.Process.Tracer, "process.tracer should be populated") &&
			assert.NotNil(t, parsed.Process.Tracer.Trace, "process.tracer.trace should be populated") {
			assertSpanFields(t, parsed.Process.Tracer.Trace, expectedSpanID, expectedTraceID, expectedAttrs, "process.tracer.trace")
		}
	} else {
		if parsed.Process.Tracer != nil {
			assert.Nil(t, parsed.Process.Tracer.Trace,
				"process.tracer.trace should be unset (event.SpanContext was zero at exec time; nothing for AddExecEntry to persist)")
		}
	}

	// (3) "process.ancestors[].tracer.trace" — populated on the fork
	// parent's PCE in fork+exec scenarios.
	if loc.onAncestor {
		var ancestorSpan *traceJSON
		for i := range parsed.Process.Ancestors {
			if parsed.Process.Ancestors[i].Tracer != nil && parsed.Process.Ancestors[i].Tracer.Trace != nil {
				ancestorSpan = parsed.Process.Ancestors[i].Tracer.Trace
				break
			}
		}
		if assert.NotNil(t, ancestorSpan,
			"at least one ancestor in process.ancestors[] should carry a serialized tracer.trace") {
			assertSpanFields(t, ancestorSpan, expectedSpanID, expectedTraceID, expectedAttrs, "ancestor.tracer.trace")
		}
	}
}

// assertSpanFields asserts the fields of a serialized span_context match the
// expected span/trace IDs and the expected attribute subset.
func assertSpanFields(t *testing.T, sc *traceJSON, expectedSpanID, expectedTraceID string, expectedAttrs map[string]string, prefix string) {
	t.Helper()
	assert.Equal(t, expectedSpanID, sc.SpanID, "%s.span_id", prefix)
	assert.Equal(t, expectedTraceID, sc.TraceID, "%s.trace_id", prefix)
	for k, v := range expectedAttrs {
		assert.Equal(t, v, sc.Attributes[k], "%s.attributes[%q]", prefix, k)
	}
}

// TestGoSpan tests Go pprof label-based span context collection.
// dd-trace-go sets goroutine labels "span id" and "local root span id" as decimal strings.
// The eBPF code traverses TLS -> runtime.g -> runtime.m -> curg -> labels to read them.
func TestGoSpan(t *testing.T) {
	SkipIfNotAvailable(t)
	skipIfNoThreadPointer(t)

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

	spanTester, err := loadSyscallTester(t, test, "span_go_tester")
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
				cmd := cmdFunc(spanTester, args, envs)
				out, err := cmd.CombinedOutput()

				if err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}

				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_go_span_rule_open")

				test.validateSpanSchema(t, event)

				jsonStr, err := test.marshalEvent(event)
				if assert.NoError(t, err, "marshalEvent") {
					assertSerializedTrace(t, jsonStr,
						strconv.FormatUint(987654321, 10),
						utils.TraceID{Lo: 123456789}.HexString(),
						nil)
				}
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
				cmd := cmdFunc(spanTester, args, []string{})
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}
				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_go_span_rule_exec")

				test.validateSpanSchema(t, event)

				// In-process exec via syscall.Exec preserves the tgid:
				// fill_span_context_go reads the goroutine's pprof labels at
				// prepare_binprm, AddExecEntry persists event.SpanContext
				// onto the new touch PCE → process.span_context populated.
				jsonStr, err := test.marshalEvent(event)
				if assert.NoError(t, err, "marshalEvent") {
					assertSerializedSpanContext(t, jsonStr,
						strconv.FormatUint(987654321, 10),
						utils.TraceID{Lo: 123456789}.HexString(),
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
				cmd := cmdFunc(spanTester, args, []string{})
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}
				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_go_span_rule_open_no_labels")

				jsonStr, err := test.marshalEvent(event)
				if assert.NoError(t, err, "marshalEvent") {
					assertNoSerializedTrace(t, jsonStr)
				}
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
				cmd := cmdFunc(spanTester, args, []string{})
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}
				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_go_span_rule_exec")

				jsonStr, err := test.marshalEvent(event)
				if assert.NoError(t, err, "marshalEvent") {
					assertNoSerializedTrace(t, jsonStr)
				}
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
				cmd := cmdFunc(spanTester, args, []string{})
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}
				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_go_span_rule_exec")

				// (1) The exec'd program (touch) has no tracer, so the raw
				// exec event SpanContext is empty by design.
				sc := event.FieldHandlers.ResolveSpanContext(event)
				assert.Equal(t, uint64(0), sc.SpanID,
					"exec event should not carry a span context: touch has no tracer")
				assert.Equal(t, "0", sc.TraceID.String(),
					"exec event should not carry a trace id: touch has no tracer")

				// (2) The immediate fork-parent in the ancestor lineage
				// should carry the parent's pprof-label span. Walk the
				// ancestor chain like newDDContextSerializer does and
				// confirm we find a PCE with the expected SpanID/TraceID.
				var foundSpan bool
				var ancestorSpanID, ancestorTraceIDLo, ancestorTraceIDHi uint64
				for pce := event.ProcessContext.Ancestor; pce != nil; pce = pce.Ancestor {
					if pce.Tracer.Trace.SpanID != 0 {
						foundSpan = true
						ancestorSpanID = pce.Tracer.Trace.SpanID
						ancestorTraceIDLo = pce.Tracer.Trace.TraceID.Lo
						ancestorTraceIDHi = pce.Tracer.Trace.TraceID.Hi
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
						utils.TraceID{Lo: parentLocalRootSpanID}.HexString(),
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
	skipIfNoThreadPointer(t)

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

	spanTester, err := loadSyscallTester(t, test, "span_go_tester")
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
				cmd := cmdFunc(spanTester, args, []string{})
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

				jsonStr, err := test.marshalEvent(event)
				if assert.NoError(t, err, "marshalEvent") {
					assertSerializedTrace(t, jsonStr,
						strconv.FormatUint(expectedSpanID, 10),
						utils.TraceID{Lo: expectedLocalRootSpanID}.HexString(),
						nil)
				}
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
				cmd := cmdFunc(spanTester, args, []string{})
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

				// In-process exec via syscall.Exec: dd-trace-go's pprof labels
				// on the locked OS thread are read by fill_span_context_go at
				// prepare_binprm, AddExecEntry persists event.SpanContext
				// onto the new touch PCE → process.span_context populated.
				jsonStr, err := test.marshalEvent(event)
				if assert.NoError(t, err, "marshalEvent") {
					assertSerializedSpanContext(t, jsonStr,
						strconv.FormatUint(expectedSpanID, 10),
						utils.TraceID{Lo: expectedLocalRootSpanID}.HexString(),
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
				cmd := cmdFunc(spanTester, args, []string{})
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}
				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_ddtrace_span_rule_open_no_span")

				jsonStr, err := test.marshalEvent(event)
				if assert.NoError(t, err, "marshalEvent") {
					assertNoSerializedTrace(t, jsonStr)
				}
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
				cmd := cmdFunc(spanTester, args, []string{})
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}
				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_ddtrace_span_rule_exec")

				jsonStr, err := test.marshalEvent(event)
				if assert.NoError(t, err, "marshalEvent") {
					assertNoSerializedTrace(t, jsonStr)
				}
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
				cmd := cmdFunc(spanTester, args, []string{})
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
				sc := event.FieldHandlers.ResolveSpanContext(event)
				assert.Equal(t, uint64(0), sc.SpanID,
					"exec event should not carry a span context: touch has no tracer")
				assert.Equal(t, "0", sc.TraceID.String(),
					"exec event should not carry a trace id: touch has no tracer")

				// (2) The immediate fork-parent in the ancestor lineage
				// should carry dd-trace-go's parent span. Walk the chain
				// the same way newDDContextSerializer does.
				var foundSpan bool
				var ancestorSpanID, ancestorTraceIDLo, ancestorTraceIDHi uint64
				for pce := event.ProcessContext.Ancestor; pce != nil; pce = pce.Ancestor {
					if pce.Tracer.Trace.SpanID != 0 {
						foundSpan = true
						ancestorSpanID = pce.Tracer.Trace.SpanID
						ancestorTraceIDLo = pce.Tracer.Trace.TraceID.Lo
						ancestorTraceIDHi = pce.Tracer.Trace.TraceID.Hi
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
						utils.TraceID{Lo: parentLocalRootSpanID}.HexString(),
						nil,
						spanLocations{onAncestor: true})
				}
			}, "test_ddtrace_span_rule_exec")
		})
	})
}

// TestOTelSpan covers OTel Thread Local Context Record span collection across
// every TLS access model a tracer can publish the record through: local-exec
// from a main executable, both dialects of general-dynamic, initial-exec and
// local-dynamic from a shared object linked at startup or dlopen'd, against
// both glibc and musl.
func TestOTelSpan(t *testing.T) {
	SkipIfNotAvailable(t)
	skipIfNoThreadPointer(t)

	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("OTel TLSDESC span test only supported on amd64 and arm64")
	}

	executable := which(t, "touch")

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_otel_span_rule_open",
			Expression: `open.file.path in [ "{{.Root}}/test-otel-span", "{{.Root}}/test-otel-span-ready" ]`,
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
			// Shared by all the exec sub-tests, which run sequentially.
			ID:         "test_otel_span_rule_exec",
			Expression: fmt.Sprintf(`exec.file.path in [ "/bin/touch", "/usr/bin/touch", "%s" ] && exec.args_flags == "reference"`, executable),
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	type otelTesterVariant struct {
		name string
		// binary and prefixArgs address the tester: the dlopen driver takes the
		// shared object to load ahead of the otel-span-* command, the others
		// take the command directly.
		binary     string
		prefixArgs []string
	}

	otelArgs := func(variant otelTesterVariant, args ...string) []string {
		return append(append([]string{}, variant.prefixArgs...), args...)
	}

	// requireOTelTesterRuns skips the leg whose environment cannot start the
	// tester. The testers link against the libc of the host that built them, so
	// the docker legs, whose image is older than the build host, reach only the
	// statically linked ones. Running the check through cmdFunc asks the very
	// environment the sub-test is about to use.
	requireOTelTesterRuns := func(t *testing.T, variant otelTesterVariant, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		t.Helper()
		if out, err := cmdFunc(variant.binary, otelArgs(variant, "check"), nil).CombinedOutput(); err != nil {
			t.Skipf("%s cannot run here: %v (output: %s)", variant.name, err, bytes.TrimSpace(out))
		}
	}

	// otelTesterSpecs mirrors the matrix the upstream profiler builds for its own
	// integration tests (processcontext/integrationtests in
	// open-telemetry/opentelemetry-ebpf-profiler#1229): one tester per TLS access
	// model the resolver has to handle, against both libcs. See
	// otel_tls_build_steps in tasks/security_agent.py for how each is built.
	type otelTesterSpec struct {
		name string
		// binary and fixture name artifacts of the syscall tester build; fixture
		// is the shared object defining otel_thread_ctx_v1, which has to sit next
		// to the binary whether the binary links it at startup or dlopens it.
		binary  string
		fixture string
		dlopen  bool
		// alpine runs the variant in an Alpine container, the only place the musl
		// testers that need a runtime loader can start.
		alpine bool
		// execCoverage adds the variant to the exec sub-tests. Carrying a span
		// across exec is independent of the access model the record was published
		// through, so only a few variants need to prove it.
		execCoverage bool
		// negatives runs the invalid-record and NULL-pointer cases on this
		// tester. They do not depend on the access model either, so they run on
		// the one tester with no toolchain or runtime requirement of its own.
		negatives bool
	}

	otelTesterSpecs := []otelTesterSpec{
		// local-exec: the variable lives in the executable's own static TLS
		// block, read from .dynsym for the dynamic build and from .symtab for
		// the fully static ones.
		{name: "exe-glibc", binary: "otel_tls_exe_glibc", execCoverage: true},
		{name: "exe-static-glibc", binary: "otel_tls_exe_static_glibc", execCoverage: true, negatives: true},
		{name: "exe-static-musl", binary: "otel_tls_exe_static_musl", execCoverage: true},
		{name: "exe-musl", binary: "otel_tls_exe_musl", alpine: true},

		// A shared object linked at startup, once per access model.
		{name: "linked-glibc", binary: "otel_tls_linked_glibc", fixture: "libotel_tls_glibc.so"},
		{name: "linked-glibc-gnu", binary: "otel_tls_linked_glibc_gnu", fixture: "libotel_tls_glibc_gnu.so", execCoverage: true},
		{name: "linked-glibc-ie", binary: "otel_tls_linked_glibc_ie", fixture: "libotel_tls_glibc_ie.so"},
		{name: "linked-glibc-ld", binary: "otel_tls_linked_glibc_ld", fixture: "libotel_tls_glibc_ld.so"},

		// The same shared objects, dlopen'd once the process is already running.
		{name: "dlopen-glibc", binary: "otel_tls_dlopen_glibc", fixture: "libotel_tls_glibc.so", dlopen: true, execCoverage: true},
		{name: "dlopen-glibc-gnu", binary: "otel_tls_dlopen_glibc", fixture: "libotel_tls_glibc_gnu.so", dlopen: true},
		{name: "dlopen-glibc-ie", binary: "otel_tls_dlopen_glibc", fixture: "libotel_tls_glibc_ie.so", dlopen: true},
		{name: "dlopen-glibc-ld", binary: "otel_tls_dlopen_glibc", fixture: "libotel_tls_glibc_ld.so", dlopen: true},

		// musl only differs from glibc once the offset comes from the DTV, which
		// it lays out its own way, so its dynamic variants are worth the
		// container they need.
		{name: "linked-musl", binary: "otel_tls_linked_musl", fixture: "libotel_tls_musl.so", alpine: true},
		{name: "linked-musl-gnu", binary: "otel_tls_linked_musl_gnu", fixture: "libotel_tls_musl_gnu.so", alpine: true},
		{name: "linked-musl-ie", binary: "otel_tls_linked_musl_ie", fixture: "libotel_tls_musl_ie.so", alpine: true},
		{name: "linked-musl-ld", binary: "otel_tls_linked_musl_ld", fixture: "libotel_tls_musl_ld.so", alpine: true},
		{name: "dlopen-musl", binary: "otel_tls_dlopen_musl", fixture: "libotel_tls_musl.so", dlopen: true, alpine: true},
		{name: "dlopen-musl-gnu", binary: "otel_tls_dlopen_musl", fixture: "libotel_tls_musl_gnu.so", dlopen: true, alpine: true},
	}

	// loadOTelTesterVariant materializes a spec's artifacts, dropping the variant
	// when the build could not produce them: the flags behind an access model are
	// GCC options a clang-only build image rejects, and the dynamically linked
	// testers need a toolchain able to link against the system libc.
	loadOTelTesterVariant := func(t *testing.T, spec otelTesterSpec) (otelTesterVariant, bool) {
		variant := otelTesterVariant{name: spec.name}

		binary, err := loadSyscallTesterArtifact(test, spec.binary)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				t.Fatal(err)
			}
			t.Logf("%s not embedded; skipping the %s OTel TLS variant", spec.binary, spec.name)
			return otelTesterVariant{}, false
		}
		variant.binary = binary

		if spec.fixture != "" {
			fixture, err := loadSyscallTesterArtifact(test, spec.fixture)
			if err != nil {
				if !errors.Is(err, fs.ErrNotExist) {
					t.Fatal(err)
				}
				t.Logf("%s not embedded; skipping the %s OTel TLS variant", spec.fixture, spec.name)
				return otelTesterVariant{}, false
			}
			if spec.dlopen {
				variant.prefixArgs = []string{fixture}
			}
		}

		return variant, true
	}

	var alpineWrapper *dockerCmdWrapper
	alpineUnavailable := false
	var negativeTester string
	var otelTesterVariants []otelTesterVariant
	var otelExecVariants []otelTesterVariant
	var alpineTesterVariants []otelTesterVariant

	for _, spec := range otelTesterSpecs {
		variant, ok := loadOTelTesterVariant(t, spec)
		if !ok {
			continue
		}

		if spec.alpine {
			if alpineWrapper == nil && !alpineUnavailable {
				alpineWrapper, err = newDockerCmdWrapper(test.Root(), test.Root(), "alpine", "")
				if err != nil {
					t.Logf("skipping the OTel TLS variants needing an Alpine container: %v", err)
					alpineUnavailable = true
				}
			}
			if alpineWrapper == nil {
				continue
			}
			alpineTesterVariants = append(alpineTesterVariants, variant)
			continue
		}

		otelTesterVariants = append(otelTesterVariants, variant)
		if spec.execCoverage {
			otelExecVariants = append(otelExecVariants, variant)
		}
		if spec.negatives {
			negativeTester = variant.binary
		}
	}

	if negativeTester == "" {
		// Producing no tester at all means the build was cross-compiling and
		// never tried (see build_otel_tls_glibc_artifacts); producing some but
		// not the one needing nothing more than a static link is an error worth
		// failing on rather than a platform this cannot cover.
		if len(otelTesterVariants) == 0 && len(alpineTesterVariants) == 0 {
			t.Skip("no OTel TLS tester embedded")
		}
		t.Fatal("otel_tls_exe_static_glibc is missing while other OTel TLS testers were embedded")
	}

	fakeTraceID128b := "136272290892501783905308705057321818530"

	assertOTelOpenSpan := func(t *testing.T, event *model.Event) {
		t.Helper()
		test.validateSpanSchema(t, event)

		assert.Equal(t, "204", strconv.FormatUint(event.SpanContext.SpanID, 10))
		assert.Equal(t, fakeTraceID128b, event.SpanContext.TraceID.String())

		assert.NotNil(t, event.SpanContext.Attributes, "attributes should be non-nil")
		assert.Equal(t, "GET", event.SpanContext.Attributes["http.method"],
			"http.method attribute should be GET")
		assert.Equal(t, "/test", event.SpanContext.Attributes["http.target"],
			"http.target attribute should be /test")
		assert.Equal(t, "will@datadoghq.com", event.SpanContext.Attributes["http.user"],
			"http.user attribute should be will@datadoghq.com")
	}

	// otelExecArgs returns the touch invocation the exec rule matches; std and
	// docker modes reach touch through different paths.
	otelExecArgs := func(kind wrapperType, testFile string) []string {
		touchPath := "/usr/bin/touch"
		if kind == stdWrapperType {
			touchPath = executable
		}
		return []string{touchPath, "--reference", "/etc/passwd", testFile}
	}

	t.Run("valid_record", func(t *testing.T) {
		for _, variant := range otelTesterVariants {
			variant := variant
			t.Run(variant.name, func(t *testing.T) {
				test.RunMultiMode(t, "open", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
					requireOTelTesterRuns(t, variant, cmdFunc)

					testFile, _, err := test.Path("test-otel-span")
					if err != nil {
						t.Fatal(err)
					}
					defer os.Remove(testFile)

					args := otelArgs(variant, "otel-span-open", fakeTraceID128b, "204", testFile)
					envs := []string{}

					test.WaitSignalFromRule(t, func() error {
						cmd := cmdFunc(variant.binary, args, envs)
						out, err := cmd.CombinedOutput()

						if err != nil {
							return fmt.Errorf("%s: %w", out, err)
						}

						return nil
					}, func(event *model.Event, rule *rules.Rule) {
						assertTriggeredRule(t, rule, "test_otel_span_rule_open")
						assertOTelOpenSpan(t, event)
					}, "test_otel_span_rule_open")
				})
			})
		}

		// A tester running in a container was started before the agent could see
		// it, so these variants publish the record behind a ready file and have
		// the resolution driven by an explicit snapshot.
		for _, variant := range alpineTesterVariants {
			variant := variant
			t.Run(variant.name, func(t *testing.T) {
				ebpfProbe, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
				if !ok {
					t.Skip("OTel TLS snapshot requires the eBPF probe")
				}

				alpineWrapper.Run(t, "open", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
					requireOTelTesterRuns(t, variant, cmdFunc)

					testFile, _, err := test.Path("test-otel-span")
					if err != nil {
						t.Fatal(err)
					}
					readyFile, _, err := test.Path("test-otel-span-ready")
					if err != nil {
						t.Fatal(err)
					}
					continueFile, _, err := test.Path("test-otel-span-continue")
					if err != nil {
						t.Fatal(err)
					}
					defer os.Remove(testFile)
					defer os.Remove(readyFile)
					defer os.Remove(continueFile)

					args := otelArgs(variant, "otel-span-open-wait", fakeTraceID128b, "204", readyFile, continueFile, testFile)
					var out bytes.Buffer
					var done chan error
					commandWaited := false

					releaseCommand := func() {
						_ = os.WriteFile(continueFile, []byte("continue"), 0o600)
					}
					waitCommand := func(timeout time.Duration) error {
						if done == nil || commandWaited {
							return nil
						}
						select {
						case err := <-done:
							commandWaited = true
							return err
						case <-time.After(timeout):
							return fmt.Errorf("timed out waiting for the %s tester", variant.name)
						}
					}
					t.Cleanup(func() {
						releaseCommand()
						if err := waitCommand(time.Second); err != nil {
							t.Logf("%s: %v", out.String(), err)
						}
					})

					err = test.getSignalFromRule(t, func() error {
						cmd := cmdFunc(variant.binary, args, []string{})
						cmd.Stdout = &out
						cmd.Stderr = &out
						if err := cmd.Start(); err != nil {
							return err
						}
						done = make(chan error, 1)
						go func() {
							done <- cmd.Wait()
						}()
						return nil
					}, func(event *model.Event, rule *rules.Rule) error {
						switch event.Open.File.PathnameStr {
						case readyFile:
							validateProcessContext(t, event)
							ebpfProbe.Resolvers.ProcessResolver.SnapshotTracer(event.PIDContext.Pid)
							releaseCommand()
							return errSkipEvent
						case testFile:
							validateProcessContext(t, event)
							assertTriggeredRule(t, rule, "test_otel_span_rule_open")
							assertOTelOpenSpan(t, event)
							return nil
						default:
							return errSkipEvent
						}
					}, "test_otel_span_rule_open")
					if err != nil {
						releaseCommand()
						_ = waitCommand(time.Second)
						t.Fatal(err)
					}
					if err := waitCommand(5 * time.Second); err != nil {
						t.Fatalf("%s: %v", out.String(), err)
					}
				})
			})
		}
	})

	t.Run("invalid_record", func(t *testing.T) {
		test.RunMultiMode(t, "open", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
			testFile, _, err := test.Path("test-otel-span-invalid")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(testFile)

			args := []string{"otel-span-open-invalid", fakeTraceID128b, "204", testFile}
			envs := []string{}

			test.WaitSignalFromRule(t, func() error {
				cmd := cmdFunc(negativeTester, args, envs)
				out, err := cmd.CombinedOutput()

				if err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}

				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_otel_span_rule_open_invalid")

				// valid=0 -> no span context.
				assert.Equal(t, uint64(0), event.SpanContext.SpanID)
				assert.Equal(t, "0", event.SpanContext.TraceID.String())
			}, "test_otel_span_rule_open_invalid")
		})
	})

	t.Run("null_pointer", func(t *testing.T) {
		test.RunMultiMode(t, "open", func(t *testing.T, _ wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
			testFile, _, err := test.Path("test-otel-span-null-ptr")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(testFile)

			args := []string{"otel-span-open-null-ptr", testFile}
			envs := []string{}

			test.WaitSignalFromRule(t, func() error {
				cmd := cmdFunc(negativeTester, args, envs)
				out, err := cmd.CombinedOutput()

				if err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}

				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, "test_otel_span_rule_open_null_ptr")

				// NULL TLS pointer -> no span context.
				assert.Equal(t, uint64(0), event.SpanContext.SpanID)
				assert.Equal(t, "0", event.SpanContext.TraceID.String())
			}, "test_otel_span_rule_open_null_ptr")
		})
	})

	t.Run("valid_record_exec", func(t *testing.T) {
		// The record is published before execv, and fill_span_context runs at
		// prepare_binprm, before the new image takes over, so the read still finds it.
		for _, variant := range otelExecVariants {
			variant := variant
			t.Run(variant.name, func(t *testing.T) {
				test.RunMultiMode(t, "exec", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
					requireOTelTesterRuns(t, variant, cmdFunc)

					testFile, _, err := test.Path("test-otel-span-exec")
					if err != nil {
						t.Fatal(err)
					}
					defer os.Remove(testFile)

					args := otelArgs(variant, append([]string{"otel-span-exec", fakeTraceID128b, "204"}, otelExecArgs(kind, testFile)...)...)

					test.WaitSignalFromRule(t, func() error {
						cmd := cmdFunc(variant.binary, args, []string{})
						if out, err := cmd.CombinedOutput(); err != nil {
							return fmt.Errorf("%s: %w", out, err)
						}
						return nil
					}, func(event *model.Event, rule *rules.Rule) {
						assertTriggeredRule(t, rule, "test_otel_span_rule_exec")

						test.validateSpanSchema(t, event)

						assert.Equal(t, "204", strconv.FormatUint(event.SpanContext.SpanID, 10))
						assert.Equal(t, fakeTraceID128b, event.SpanContext.TraceID.String())

						expectedHi, expectedLo, ok := splitTraceID(fakeTraceID128b)
						if !assert.True(t, ok, "splitTraceID") {
							return
						}
						jsonStr, err := test.marshalEvent(event)
						if assert.NoError(t, err, "marshalEvent") {
							assertSerializedSpanContext(t, jsonStr, "204",
								utils.TraceID{Hi: expectedHi, Lo: expectedLo}.HexString(),
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
		}
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
				cmd := cmdFunc(negativeTester, args, []string{})
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
				cmd := cmdFunc(negativeTester, args, []string{})
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
		// At sched_process_fork the probe still runs in the parent's context, so the
		// parent's record is captured onto the child's ProcessCacheEntry. The exec'd
		// touch carries no record of its own, so the span only surfaces by walking
		// the ancestor chain.
		for _, variant := range otelExecVariants {
			variant := variant
			t.Run(variant.name, func(t *testing.T) {
				test.RunMultiMode(t, "exec", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
					requireOTelTesterRuns(t, variant, cmdFunc)

					testFile, _, err := test.Path("test-otel-span-fork-exec")
					if err != nil {
						t.Fatal(err)
					}
					defer os.Remove(testFile)

					args := otelArgs(variant, append([]string{"otel-span-fork-exec", fakeTraceID128b, "204"}, otelExecArgs(kind, testFile)...)...)

					test.WaitSignalFromRule(t, func() error {
						cmd := cmdFunc(variant.binary, args, []string{})
						if out, err := cmd.CombinedOutput(); err != nil {
							return fmt.Errorf("%s: %w", out, err)
						}
						return nil
					}, func(event *model.Event, rule *rules.Rule) {
						assertTriggeredRule(t, rule, "test_otel_span_rule_exec")

						assert.Equal(t, uint64(0), event.SpanContext.SpanID,
							"exec event should not carry a span context: touch has no tracer")
						assert.Equal(t, "0", event.SpanContext.TraceID.String(),
							"exec event should not carry a trace id: touch has no tracer")

						var foundAncestor *model.ProcessCacheEntry
						for pce := event.ProcessContext.Ancestor; pce != nil; pce = pce.Ancestor {
							if pce.Tracer.Trace.SpanID != 0 {
								foundAncestor = pce
								break
							}
						}
						if assert.NotNil(t, foundAncestor,
							"an ancestor should carry the parent's OTel TLS span captured at fork time") {
							assert.Equal(t, uint64(204), foundAncestor.Tracer.Trace.SpanID,
								"fork-parent ancestor SpanID should equal the OTel record span_id")
							assert.Equal(t, fakeTraceID128b, foundAncestor.Tracer.Trace.TraceID.String(),
								"fork-parent ancestor TraceID should equal the OTel record trace_id")
						}

						expectedHi, expectedLo, ok := splitTraceID(fakeTraceID128b)
						if !assert.True(t, ok, "splitTraceID") {
							return
						}
						jsonStr, err := test.marshalEvent(event)
						if assert.NoError(t, err, "marshalEvent") {
							assertSerializedSpanContext(t, jsonStr, "204",
								utils.TraceID{Hi: expectedHi, Lo: expectedLo}.HexString(), nil,
								spanLocations{onAncestor: true})
						}
					}, "test_otel_span_rule_exec")
				})
			})
		}
	})
}
