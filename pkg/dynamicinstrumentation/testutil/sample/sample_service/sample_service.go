// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Sample-service is a simple program with lots of functions to test GoDI against
package main

import (
	"net"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/testutil/sample"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

func main() {
	tracerEnabled := os.Getenv("DD_SERVICE") != "" &&
		os.Getenv("DD_DYNAMIC_INSTRUMENTATION_ENABLED") != "" &&
		os.Getenv("DD_DYNAMIC_INSTRUMENTATION_OFFLINE") == ""

	log.Info("Starting sample process with tracerEnabled=", tracerEnabled)

	if tracerEnabled {
		ddAgentHost := os.Getenv("DD_AGENT_HOST")
		if ddAgentHost == "" {
			ddAgentHost = "localhost"
		}
		// Start the tracer and defer the Stop method.
		tracer.Start(tracer.WithAgentAddr(net.JoinHostPort(ddAgentHost, "8126")),
			tracer.WithDebugMode(true))
	}

	ticker := time.NewTicker(time.Second / 2)
	for range ticker.C {
		sample.ExecuteOther()
		sample.ExecuteBasicFuncs()
		sample.ExecuteMultiParamFuncs()
		sample.ExecuteStringFuncs()
		sample.ExecuteArrayFuncs()
		sample.ExecuteSliceFuncs()
		sample.ExecuteStructFuncs()
		sample.ExecuteStackAndInlining()
		sample.ExecutePointerFuncs()
		sample.ExecuteComplexFuncs()

		// unsupported for MVP, should not cause crashes
		sample.ExecuteGenericFuncs()
		sample.ExecuteMapFuncs()
		sample.ExecuteInterfaceFuncs()
		go sample.Return_goroutine_id()
	}
}
