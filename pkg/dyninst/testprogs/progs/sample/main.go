// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Sample-service is a simple program with lots of functions to test GoDI against
package main

import (
	"bufio"
	"os"
	"time"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"

	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs/progs/sample/lib"
	lib_v2 "github.com/DataDog/datadog-agent/pkg/dyninst/testprogs/progs/sample/lib.v2"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs/progs/sample/lib2"
)

func main() {
	// Disable 128-bit trace id generation so the upper bits of the trace id
	// are zero rather than time-based — keeps the testTakeContext snapshot
	// trace_id deterministic across runs (lower 64 bits come from
	// tracer.WithSpanID in executeContextFuncs).
	os.Setenv("DD_TRACE_128_BIT_TRACEID_GENERATION_ENABLED", "false")
	// Default to "sample-service" for dyninst tests, but let DD_SERVICE win so
	// demo deployments can report under their own service name.
	service := os.Getenv("DD_SERVICE")
	if service == "" {
		service = "sample-service"
	}
	tracer.Start(tracer.WithService(service))

	if os.Getenv("DD_SAMPLE_LOOP") == "" {
		// Wait for input before executing functions to allow time for uprobe attachment
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		runAll()
		return
	}

	// Long-running mode used by the debugger demo deployment.
	ticker := time.NewTicker(500 * time.Millisecond)
	for range ticker.C {
		span := tracer.StartSpan("demo-round")
		runAll()
		span.Finish()
	}
}

func runAll() {
	executeOther()
	executeBasicFuncs()
	executeMultiParamFuncs()
	executeStringFuncs()
	executeArrayFuncs()
	executeSliceFuncs()
	executeStructFuncs()
	executeStack()
	executeInlined()
	executePointerFuncs()
	executeComplexFuncs()
	lib.Foo()
	lib_v2.FooV2()
	var t lib_v2.V2Type
	t.MyMethod()
	lib_v2.UseV2GenericBox()
	var it iterExample
	it.rangeOverIterator()
	lib.InlinedFunc()
	lib2.UseGenericsWithFloat64()

	executeContinuationFuncs()
	executeContinuationStringFuncs()
	executeTimeFuncs()

	// unsupported for MVP, should not cause failures
	executeEsoteric()
	executeGenericFuncs()
	executeMapFuncs()
	executeInterfaceFuncs()
	executeReturns()
	executeContextFuncs()
	go returnGoroutineId()
	executeContextImplFuncs()
}
