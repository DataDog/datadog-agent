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
	executeStackAndInlining()
	executePointerFuncs()
	executeComplexFuncs()

	// unsupported for MVP, should not cause crashes
	executeGenericFuncs()
	executeMapFuncs()
	executeInterfaceFuncs()
	go returnGoroutineId()

}
