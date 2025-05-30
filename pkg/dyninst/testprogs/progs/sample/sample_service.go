// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Sample-service is a simple program with lots of functions to test GoDI against
package main

import (
	"bufio"
	"log"
	"net"
	"os"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
)

func main() {

	tracerEnabled := os.Getenv("DD_SERVICE") != "" &&
		os.Getenv("DD_DYNAMIC_INSTRUMENTATION_ENABLED") != "" &&
		os.Getenv("DD_DYNAMIC_INSTRUMENTATION_OFFLINE") == ""

	log.Println("Starting sample process with tracerEnabled=", tracerEnabled)

	if tracerEnabled {
		ddAgentHost := os.Getenv("DD_AGENT_HOST")
		if ddAgentHost == "" {
			ddAgentHost = "localhost"
		}
		// Start the tracer and defer the Stop method.
		err := tracer.Start(tracer.WithAgentAddr(net.JoinHostPort(ddAgentHost, "8126")),
			tracer.WithDebugMode(true))
		if err != nil {
			log.Printf("error starting tracer: %s", err)
		}
		defer tracer.Stop()
	}

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
