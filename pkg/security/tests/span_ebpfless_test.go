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
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/utils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// TestOTelSpanEBPFLess covers OTel Thread Local Context Record span collection
// by the ptracer, against the same testers TestOTelSpan drives.
//
// The ptracer stops the tracee on every syscall it reports, so the record and
// the process context are already published, and still in place, when it reads
// them: unlike the eBPF probe nothing has to resolve the tester before the
// record is published, and the tester runs without a continue file.
func TestOTelSpanEBPFLess(t *testing.T) {
	SkipIfNotAvailable(t)

	if !ebpfLessEnabled {
		t.Skip("ebpfless specific")
	}

	executable := which(t, "touch")

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_otel_span_ebpfless_rule_open",
			Expression: `open.file.path == "{{.Root}}/test-otel-span-ebpfless"`,
		},
		{
			ID:         "test_otel_span_ebpfless_rule_open_invalid",
			Expression: `open.file.path == "{{.Root}}/test-otel-span-ebpfless-invalid"`,
		},
		{
			ID:         "test_otel_span_ebpfless_rule_open_null_ptr",
			Expression: `open.file.path == "{{.Root}}/test-otel-span-ebpfless-null-ptr"`,
		},
		{
			ID:         "test_otel_span_ebpfless_rule_fork_open",
			Expression: `open.file.path == "{{.Root}}/test-otel-span-ebpfless-fork-open"`,
		},
		{
			ID:         "test_otel_span_ebpfless_rule_exec_open",
			Expression: `open.file.path == "{{.Root}}/test-otel-span-ebpfless-exec-open"`,
		},
		{
			// Shared by all the exec sub-tests, which run sequentially.
			ID:         "test_otel_span_ebpfless_rule_exec",
			Expression: fmt.Sprintf(`exec.file.path in [ "/bin/touch", "/usr/bin/touch", "%s" ] && exec.args_flags == "reference"`, executable),
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	var negativeTester *otelTesterVariant
	var otelTesterVariants []otelTesterVariant
	var otelExecVariants []otelTesterVariant

	for _, spec := range otelTesterSpecs {
		variant, ok := loadOTelTesterVariant(t, test, spec)
		if !ok {
			continue
		}

		otelTesterVariants = append(otelTesterVariants, variant)
		if spec.execCoverage {
			otelExecVariants = append(otelExecVariants, variant)
		}
		if spec.negatives {
			negativeTester = &variant
		}
	}

	if negativeTester == nil {
		if len(otelTesterVariants) == 0 {
			t.Skip("no OTel TLS tester embedded")
		}
		t.Fatal("otel_tls_exe_static_glibc is missing while other OTel TLS testers were embedded")
	}

	expectedHi, expectedLo, ok := splitTraceID(otelFakeTraceID128b)
	if !ok {
		t.Fatalf("splitTraceID(%s)", otelFakeTraceID128b)
	}
	expectedTraceIDHex := utils.TraceID{Hi: expectedHi, Lo: expectedLo}.HexString()

	assertNoSpan := func(t *testing.T, event *model.Event) {
		t.Helper()
		assert.Equal(t, uint64(0), event.SpanContext.SpanID)
		assert.Equal(t, "0", event.SpanContext.TraceID.String())
	}

	// runOTel runs one command of variant, whose arguments args builds from
	// the path of the file the sub-test watches, and checks the event ruleID
	// reports.
	runOTel := func(t *testing.T, mode string, variant otelTesterVariant, fileName, ruleID string,
		args func(kind wrapperType, testFile string) []string,
		check func(t *testing.T, event *model.Event)) {
		t.Helper()

		test.RunMultiMode(t, mode, func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
			requireOTelTesterRuns(t, variant, cmdFunc)

			testFile, _, err := test.Path(fileName)
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(testFile)

			test.WaitSignalFromRule(t, func() error {
				if out, err := test.runSpanTester(cmdFunc, variant.binary, args(kind, testFile)); err != nil {
					return fmt.Errorf("%s: %w", out, err)
				}
				return nil
			}, func(event *model.Event, rule *rules.Rule) {
				assertTriggeredRule(t, rule, ruleID)
				check(t, event)
			}, ruleID)
		})
	}

	t.Run("valid_record", func(t *testing.T) {
		for _, variant := range otelTesterVariants {
			t.Run(variant.name, func(t *testing.T) {
				runOTel(t, "open", variant, "test-otel-span-ebpfless", "test_otel_span_ebpfless_rule_open",
					func(_ wrapperType, testFile string) []string {
						return otelArgs(variant, "otel-span-open", otelFakeTraceID128b, "204", testFile)
					},
					func(t *testing.T, event *model.Event) {
						assertOTelRecordSpan(t, test, event)

						jsonStr, err := test.marshalEvent(event)
						if assert.NoError(t, err, "marshalEvent") {
							assertSerializedTrace(t, jsonStr, "204", expectedTraceIDHex, otelExpectedAttrs)
						}
					})
			})
		}
	})

	t.Run("invalid_record", func(t *testing.T) {
		runOTel(t, "open", *negativeTester, "test-otel-span-ebpfless-invalid", "test_otel_span_ebpfless_rule_open_invalid",
			func(_ wrapperType, testFile string) []string {
				return []string{"otel-span-open-invalid", otelFakeTraceID128b, "204", testFile}
			},
			assertNoSpan)
	})

	t.Run("null_pointer", func(t *testing.T) {
		runOTel(t, "open", *negativeTester, "test-otel-span-ebpfless-null-ptr", "test_otel_span_ebpfless_rule_open_null_ptr",
			func(_ wrapperType, testFile string) []string {
				return []string{"otel-span-open-null-ptr", testFile}
			},
			assertNoSpan)
	})

	t.Run("valid_record_exec", func(t *testing.T) {
		// The record is published before execv, and the ptracer reads it at the
		// execve entry, before the new image replaces the address space holding it.
		for _, variant := range otelExecVariants {
			t.Run(variant.name, func(t *testing.T) {
				runOTel(t, "exec", variant, "test-otel-span-ebpfless-exec", "test_otel_span_ebpfless_rule_exec",
					func(kind wrapperType, testFile string) []string {
						return otelArgs(variant, append([]string{"otel-span-exec", otelFakeTraceID128b, "204"}, otelTouchArgs(kind, executable, testFile)...)...)
					},
					func(t *testing.T, event *model.Event) {
						assertOTelRecordSpan(t, test, event)

						jsonStr, err := test.marshalEvent(event)
						if assert.NoError(t, err, "marshalEvent") {
							assertSerializedSpanContext(t, jsonStr, "204", expectedTraceIDHex, otelExpectedAttrs,
								spanLocations{onTopLevelProcess: true})
						}
					})
			})
		}
	})

	t.Run("invalid_record_exec", func(t *testing.T) {
		runOTel(t, "exec", *negativeTester, "test-otel-span-ebpfless-exec-invalid", "test_otel_span_ebpfless_rule_exec",
			func(kind wrapperType, testFile string) []string {
				return append([]string{"otel-span-exec-invalid", otelFakeTraceID128b, "204"}, otelTouchArgs(kind, executable, testFile)...)
			},
			assertNoSpan)
	})

	t.Run("null_pointer_exec", func(t *testing.T) {
		runOTel(t, "exec", *negativeTester, "test-otel-span-ebpfless-exec-null-ptr", "test_otel_span_ebpfless_rule_exec",
			func(kind wrapperType, testFile string) []string {
				return append([]string{"otel-span-exec-null-ptr"}, otelTouchArgs(kind, executable, testFile)...)
			},
			assertNoSpan)
	})

	t.Run("fork_exec_propagates_to_child", func(t *testing.T) {
		// The child execs with the parent's record still in its copy of the
		// address space, so the exec event reports it, and the fork leaves the
		// parent's span in the lineage of the exec'd image.
		for _, variant := range otelExecVariants {
			t.Run(variant.name, func(t *testing.T) {
				runOTel(t, "exec", variant, "test-otel-span-ebpfless-fork-exec", "test_otel_span_ebpfless_rule_exec",
					func(kind wrapperType, testFile string) []string {
						return otelArgs(variant, append([]string{"otel-span-fork-exec", otelFakeTraceID128b, "204"}, otelTouchArgs(kind, executable, testFile)...)...)
					},
					func(t *testing.T, event *model.Event) {
						assert.Equal(t, uint64(204), event.SpanContext.SpanID,
							"exec event should carry the fork parent's OTel record span_id")
						assert.Equal(t, otelFakeTraceID128b, event.SpanContext.TraceID.String(),
							"exec event should carry the fork parent's OTel record trace_id")

						jsonStr, err := test.marshalEvent(event)
						if assert.NoError(t, err, "marshalEvent") {
							assertSerializedSpanContext(t, jsonStr, "204", expectedTraceIDHex, nil,
								spanLocations{onTopLevelProcess: true, onAncestor: true})
						}
					})
			})
		}
	})

	t.Run("exec_propagates_to_later_events", func(t *testing.T) {
		// touch, which publishes no record of its own, opens the watched file
		// after the exec: the span reaches that open event only through the
		// process entry the exec stamped.
		runOTel(t, "open", *negativeTester, "test-otel-span-ebpfless-exec-open", "test_otel_span_ebpfless_rule_exec_open",
			func(kind wrapperType, testFile string) []string {
				return append([]string{"otel-span-exec", otelFakeTraceID128b, "204"}, otelTouchArgs(kind, executable, testFile)...)
			},
			func(t *testing.T, event *model.Event) {
				assert.Equal(t, uint64(0), event.SpanContext.SpanID,
					"open event should not carry a span context: touch has no tracer")

				jsonStr, err := test.marshalEvent(event)
				if assert.NoError(t, err, "marshalEvent") {
					assertSerializedSpanContext(t, jsonStr, "204", expectedTraceIDHex, nil,
						spanLocations{onTopLevelProcess: true})
				}
			})
	})

	t.Run("fork_no_exec", func(t *testing.T) {
		// The forked child publishes a record of its own before its open: the
		// child's span id, not the parent's, has to be reported.
		const childSpanID = "999"

		for _, variant := range otelExecVariants {
			t.Run(variant.name, func(t *testing.T) {
				runOTel(t, "open", variant, "test-otel-span-ebpfless-fork-open", "test_otel_span_ebpfless_rule_fork_open",
					func(_ wrapperType, testFile string) []string {
						return otelArgs(variant, "otel-span-fork-open", otelFakeTraceID128b, "204", childSpanID, testFile)
					},
					func(t *testing.T, event *model.Event) {
						assert.Equal(t, childSpanID, strconv.FormatUint(event.SpanContext.SpanID, 10),
							"the child's own span id should be read, not the parent's")
						assert.Equal(t, otelFakeTraceID128b, event.SpanContext.TraceID.String())
					})
			})
		}
	})
}
