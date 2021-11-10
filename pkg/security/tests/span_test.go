// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests

package tests

import (
	"fmt"
	"os"
	"syscall"
	"testing"
	"unsafe"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
)

func TestSpan(t *testing.T) {
	executable := which("touch")

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_span_rule_open",
			Expression: `open.file.path == "{{.Root}}/test-span"`,
		},
		{
			ID:         "test_span_rule_exec",
			Expression: fmt.Sprintf(`exec.file.path == "%s"`, executable),
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	var tls [200]uint64
	for i := range tls {
		tls[i] = 0
	}

	req := sprobe.ERPCRequest{
		OP: sprobe.RegisterSpanTLSOP,
	}

	// format, max threads, base ptr
	model.ByteOrder.PutUint64(req.Data[0:8], 0)
	model.ByteOrder.PutUint64(req.Data[8:16], uint64(len(tls)/2))
	model.ByteOrder.PutUint64(req.Data[16:24], uint64(uintptr(unsafe.Pointer(&tls))))

	erpc, err := sprobe.NewERPC()
	if err != nil {
		t.Fatal(err)
	}
	if err := erpc.Request(&req); err != nil {
		t.Fatal(err)
	}

	t.Run("open", func(t *testing.T) {
		offset := (syscall.Gettid() % (len(tls) / 2)) * 2
		tls[offset] = 123
		tls[offset+1] = 456

		test.WaitSignal(t, func() error {
			testFile, _, err := test.Create("test-span")
			if err != nil {
				return err
			}
			return os.Remove(testFile)
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_span_rule_open")

			if !validateSpanSchema(t, event) {
				t.Error(event.String())
			}

			assert.Equal(t, uint64(123), event.SpanContext.SpanID)
			assert.Equal(t, uint64(456), event.SpanContext.TraceID)
		})
	})

	t.Run("exec", func(t *testing.T) {
		syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
		if err != nil {
			if _, ok := err.(ErrUnsupportedArch); ok {
				t.Skip(err)
			} else {
				t.Fatal(err)
			}
		}

		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(t, syscallTester, "span-exec", "104", "204", executable, "/tmp/test_span_rule_exec")
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
