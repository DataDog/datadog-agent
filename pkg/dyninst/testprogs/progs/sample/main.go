// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Sample-service is a simple program with lots of functions to test GoDI against
package main

import (
	"bufio"
	"os"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"

	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs/progs/sample/lib"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs/progs/sample/lib.v2"
)

func main() {
	tracer.Start(tracer.WithService("sample-service"))

	// Wait for input before executing functions to allow time for uprobe attachment
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()

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

	// unsupported for MVP, should not cause failures
	executeEsoteric()
	executeGenericFuncs()
	executeMapFuncs()
	executeInterfaceFuncs()
	go returnGoroutineId()
}
