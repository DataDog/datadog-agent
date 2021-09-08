// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests

package tests

import (
	"os"
	"syscall"
	"testing"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/model"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/stretchr/testify/assert"
)

func TestSpan(t *testing.T) {
	ruleDef := &rules.RuleDefinition{
		ID:         "test_span_rule",
		Expression: `open.file.path == "{{.Root}}/test-span"`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{ruleDef}, testOpts{})
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

	offset := (syscall.Gettid() % (len(tls) / 2)) * 2
	tls[offset] = 123
	tls[offset+1] = 456

	err = test.GetSignal(t, func() error {
		testFile, _, err := test.Create("test-span")
		os.Remove(testFile)
		return err
	}, func(event *sprobe.Event, rule *rules.Rule) {
		assertTriggeredRule(t, rule, "test_span_rule")

		if !validateSpanSchema(t, event) {
			t.Fatal(event.String())
		}

		assert.Equal(t, uint64(123), event.SpanContext.SpanID)
		assert.Equal(t, uint64(456), event.SpanContext.TraceID)
	})
	if err != nil {
		t.Error(err)
	}
}
