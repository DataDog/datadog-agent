// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Sample-service is a simple program with lots of functions to test GoDI against
package main

import (
	"bufio"
	"os"
)

func main() {

	// Wait for input before executing functions to allow time for uprobe attachment
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()

	ExecuteOther()
	ExecuteBasicFuncs()
	ExecuteMultiParamFuncs()
	ExecuteStringFuncs()
	ExecuteArrayFuncs()
	ExecuteSliceFuncs()
	ExecuteStructFuncs()
	ExecuteStackAndInlining()
	ExecutePointerFuncs()
	ExecuteComplexFuncs()

	// unsupported for MVP, should not cause crashes
	ExecuteGenericFuncs()
	ExecuteMapFuncs()
	ExecuteInterfaceFuncs()
	go Return_goroutine_id()

}
