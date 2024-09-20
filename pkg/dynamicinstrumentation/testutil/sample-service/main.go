// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"log"
	"net"
	"os"
	"time"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
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
		tracer.Start(tracer.WithAgentAddr(net.JoinHostPort(ddAgentHost, "8126")),
			tracer.WithDebugMode(true))
	}

	go startHTTPServer()

	ticker := time.NewTicker(time.Second / 2)
	for range ticker.C {
		executeOther()
		executeBasicFuncs()
		executeMultiParamFuncs()
		executeStringFuncs()
		executeArrayFuncs()
		executeSliceFuncs()
		executeStructFuncs()
		executeStackAndInlining()
		executePointerFuncs()
		sendHTTPRequest()

		// unsupported for MVP, should not cause crashes
		executeGenericFuncs()
		executeMapFuncs()
		executeInterfaceFuncs()
		go return_goroutine_id()
	}
}
